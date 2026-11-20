#!/usr/bin/env python3
"""Export the desktop AutoMix weights as iOS 17 Core ML model packages on macOS."""

import argparse
import copy
import hashlib
import json
from pathlib import Path
import tempfile
from types import MethodType, SimpleNamespace

import coremltools as ct
from einops import rearrange
import numpy as np
from rotary_embedding_torch import RotaryEmbedding
import torch
from beat_this.model.roformer import Attention

import runner


BEAT_WINDOW = 1500


def rotation_phases(layer, positions, seq_len=None, offset=0):
    # Expand the rotary outer product as a broadcast, which Core ML can lower.
    phases = positions.to(layer.freqs.dtype).unsqueeze(-1) * layer.freqs
    return torch.repeat_interleave(phases, 2, dim=-1)


def masked_attention(layer, features, padding_bias=None):
    features = layer.norm(features)
    q, k, v = rearrange(layer.to_qkv(features), 'b t (qkv h d) -> qkv b h t d', qkv=3, h=layer.heads)
    q = layer.rotary_embed.rotate_queries_or_keys(q)
    k = layer.rotary_embed.rotate_queries_or_keys(k)
    scores = (q * layer.scale) @ k.transpose(-1, -2)
    if padding_bias is not None:
        scores = scores + padding_bias
    hidden = torch.softmax(scores, dim=-1) @ v
    if layer.to_gates is not None:
        hidden = hidden * rearrange(layer.to_gates(features), 'b t h -> b h t 1').sigmoid()
    return layer.to_out(rearrange(hidden, 'b h t d -> b t (h d)'))


class MaskedBeat(torch.nn.Module):
    """Use one static model for all windows while preserving short-window edges."""

    def __init__(self, model):
        super().__init__()
        self.network = model
        for layer in model.modules():
            if isinstance(layer, RotaryEmbedding):
                layer.forward = MethodType(rotation_phases, layer)
            if isinstance(layer, Attention):
                layer.forward = MethodType(masked_attention, layer)

    def forward(self, mel, valid):
        # Ignore the extra frames in temporal self-attention. Before each
        # temporal convolution, zero them so the valid edge sees its normal pad.
        bias = -10000 * (1 - valid[:, None, None, :])
        frontend = self.network.frontend
        stem = frontend.stem
        x = stem.bn1d(stem.rearrange_tf(mel)) * valid[:, None, :]
        x = stem.activation(stem.bn2d(stem.conv2d(stem.add_channel(x))))
        for block in frontend.blocks:
            partial = block.partial
            x = rearrange(x, 'b c f t -> (b t) f c')
            x = x + partial.attnF(x)
            x = x + partial.ffF(x)
            x = rearrange(x, '(b t) f c -> (b f) t c', b=1)
            x = x + partial.attnT(x, bias)
            x = x + partial.ffT(x)
            x = rearrange(x, '(b f) t c -> b c f t', b=1)
            x = block.activation(block.norm(block.conv2d(x * valid[:, None, None, :])))
        x = frontend.linear(frontend.concat(x))
        body = self.network.transformer_blocks
        for attention, feed_forward in body.layers:
            x = x + attention(x, bias)
            x = x + feed_forward(x)
        prediction = self.network.task_heads(body.norm(x))
        return prediction['beat'], prediction['downbeat']


class KeyFeatures(torch.nn.Module):
    def __init__(self, vqt):
        super().__init__()
        self.vqt = vqt

    def forward(self, pcm):
        # Inference always uses CropCQT(84) with zero transposition.
        return self.vqt(pcm)[:, :, :84, :]


class MeanOverTime(torch.nn.Module):
    def forward(self, features):
        return features.mean(dim=-1, keepdim=True)


def normalize_per_sample(features, _shape):
    # ChromaNet uses non-affine layer_norm over every axis except batch.
    dims = (1, 2, 3)
    centered = features - features.mean(dim=dims, keepdim=True)
    variance = centered.square().mean(dim=dims, keepdim=True)
    return centered * torch.rsqrt(variance + 1e-5)


def export(model, example, inputs, outputs, path):
    traced = torch.jit.trace(model.eval(), example, check_trace=False)
    package = ct.convert(
        traced,
        convert_to='mlprogram',
        minimum_deployment_target=ct.target.iOS17,
        compute_units=ct.ComputeUnit.CPU_ONLY,
        compute_precision=ct.precision.FLOAT32,
        inputs=[ct.TensorType(name=name, shape=shape, dtype=np.float32) for name, shape in inputs],
        outputs=[ct.TensorType(name=name, dtype=np.float32) for name in outputs],
    )
    package.save(str(path))
    return package


def beat_inputs(mel):
    frames = mel.shape[1]
    padded = np.zeros((1, BEAT_WINDOW, 128), dtype=np.float32)
    padded[:, :frames, :] = mel.numpy()
    valid = np.zeros((1, BEAT_WINDOW), dtype=np.float32)
    valid[:, :frames] = 1
    return {'mel': padded, 'valid': valid}


def export_models(models, output):
    beat, (vqt, chromanet, crop), (vocal, means, inv_std) = models
    rng = torch.Generator().manual_seed(73)
    mel = torch.zeros(1, BEAT_WINDOW, 128)
    valid = torch.ones(1, BEAT_WINDOW)
    core_beat = export(
        MaskedBeat(copy.deepcopy(beat.model)), (mel, valid),
        [('mel', mel.shape), ('valid', valid.shape)],
        ['beat', 'downbeat'], output / 'BeatThis.mlpackage',
    )
    for frames in (413, 1117, BEAT_WINDOW):
        mel = torch.randn(1, frames, 128, generator=rng)
        prediction = core_beat.predict(beat_inputs(mel))
        for name, expected in beat.model(mel).items():
            np.testing.assert_allclose(prediction[name][:, :frames], expected.numpy(), atol=8e-5, rtol=2e-5)

    fbank = torch.randn(1, 1000, 80, generator=rng)
    core_vocal = export(
        vocal, fbank, [('fbank', (1, ct.RangeDim(2, 30000, default=1000), 80))],
        ['probabilities'], output / 'FireRed.mlpackage',
    )
    for frames in (443, 1000, 30000):
        fbank = torch.randn(1, frames, 80, generator=rng)
        prediction = core_vocal.predict({'fbank': fbank.numpy()})['probabilities']
        np.testing.assert_allclose(prediction, vocal(fbank).numpy(), atol=4e-5, rtol=1e-4)
    (output / 'FireRedCMVN.json').write_text(json.dumps({'means': means.tolist(), 'inverseStd': inv_std.tolist()}) + '\n')

    pcm = torch.rand(1, 1, 22050, generator=rng) * 2 - 1
    core_vqt = export(KeyFeatures(vqt), pcm, [('pcm', pcm.shape)], ['chroma'], output / 'SKeyFeatures.mlpackage')
    prediction = core_vqt.predict({'pcm': pcm.numpy()})['chroma']
    np.testing.assert_allclose(prediction, crop(vqt(pcm), torch.zeros(1)).numpy(), atol=2e-3, rtol=1e-4)

    classifier = copy.deepcopy(chromanet)
    classifier.global_average_pool = MeanOverTime()
    for layer in (*classifier.time_downsampling_blocks, *classifier.convnext_blocks):
        layer.norm = normalize_per_sample
    chroma = torch.rand(1, 1, 84, 440, generator=rng)
    core_key = export(
        classifier, chroma, [('chroma', (1, 1, 84, ct.RangeDim(128, 52800, default=440)))],
        ['keys'], output / 'SKeyClassify.mlpackage',
    )
    for frames in (128, 440, 1301):
        chroma = torch.rand(1, 1, 84, frames, generator=rng)
        prediction = core_key.predict({'chroma': chroma.numpy()})['keys']
        np.testing.assert_allclose(prediction, chromanet(chroma).numpy(), atol=1e-4)

    def predict_beat(mel):
        frames = mel.shape[1]
        pred = core_beat.predict(beat_inputs(mel))
        return {name: torch.from_numpy(values[:, :frames]) for name, values in pred.items()}

    def predict(model, key, result):
        return lambda value: torch.from_numpy(model.predict({key: value.numpy()})[result])

    return (
        SimpleNamespace(model=predict_beat),
        (predict(core_vqt, 'pcm', 'chroma'), predict(core_key, 'chroma', 'keys'), lambda x, _: x),
        (predict(core_vocal, 'fbank', 'probabilities'), means, inv_std),
    )


@torch.no_grad()
def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('checkpoints', type=Path, help='directory prepared by setup-smart-fades.py')
    parser.add_argument('output', type=Path, help='new directory for the verified Core ML packages')
    parser.add_argument('--pcm', type=Path, action='append', default=[], help='48 kHz mono Float32 LE audio to compare end to end')
    args = parser.parse_args()
    if args.output.exists():
        parser.error('output directory already exists')

    pinned = json.loads((runner.ROOT / 'models.json').read_text())
    for name, info in pinned.items():
        with (args.checkpoints / name).open('rb') as checkpoint:
            if hashlib.file_digest(checkpoint, 'sha256').hexdigest() != info['sha256']:
                raise ValueError(f'Invalid checkpoint SHA-256: {name}')
    runner.MODEL_ROOT = args.checkpoints.resolve()
    reference = runner.load_models()
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(dir=args.output.parent, prefix='.coreml-') as temporary:
        output = Path(temporary) / 'models'
        output.mkdir()
        converted = export_models(reference, output)
        for pcm in args.pcm:
            expected = runner.analyze(pcm, 48000, models=reference)
            actual = runner.analyze(pcm, 48000, models=converted)
            for field in ('beats', 'downbeats', 'bpm', 'beats_per_bar', 'key', 'mode'):
                if actual[field] != expected[field]:
                    raise ValueError(f'{pcm.name}: {field} differs from desktop analysis')
            np.testing.assert_allclose(actual['vocal_activity'], expected['vocal_activity'], atol=4e-5, rtol=1e-4)
            print(f'{pcm.name}: matched {len(actual["beats"])} beats, {len(actual["downbeats"])} downbeats, '
                  f'{actual["bpm"]} BPM, {actual["key"]} {actual["mode"]}, and vocal activity', flush=True)
        output.rename(args.output)
    print(f'Exported and verified {args.output}')


if __name__ == '__main__':
    main()
