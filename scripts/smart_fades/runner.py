#!/usr/bin/env python3
"""Local, bounded Smart Fades inference and the pinned MA transition planner.

No audio output, network access, account data or server process is involved.
The Go player supplies decoded mono PCM and consumes validated JSON.
"""
from __future__ import annotations

import argparse
import contextlib
from dataclasses import asdict
from enum import Enum
import json
import logging
import os
from pathlib import Path
import sys
from types import SimpleNamespace

VERSION = 1
ROOT = Path(__file__).resolve().parent
MODEL_ROOT = ROOT / 'models'
if not MODEL_ROOT.is_dir():
    MODEL_ROOT = ROOT.parents[1] / 'build/smart-fades-models'


def load_models():
    import torch
    from beat_this.inference import Spect2Frames
    from inference.resources import skey_model
    from inference import vocal_activity
    for name in ('beat-this-small0.ckpt', 'skey.pt', 'firered_aed.pt', 'firered_aed_cmvn.npz'):
        if not (MODEL_ROOT / name).is_file():
            raise FileNotFoundError('Missing bundled model: ' + name)
    skey_model._CHECKPOINT_PATH = MODEL_ROOT / 'skey.pt'
    vocal_activity._MODEL_PATH = MODEL_ROOT / 'firered_aed.pt'
    vocal_activity._CMVN_PATH = MODEL_ROOT / 'firered_aed_cmvn.npz'
    torch.set_num_threads(2)
    torch.set_num_interop_threads(1)
    # A missing checkpoint is an error; playback never downloads models.
    beat = Spect2Frames(checkpoint_path=str(MODEL_ROOT / 'beat-this-small0.ckpt'), device='cpu')
    skey = skey_model.load_skey_components()
    vocal = vocal_activity.load_firered_components()
    return beat, skey, vocal


def analyze(path: Path, rate: int):
    import asyncio
    import numpy as np
    import soxr
    import torch
    import torchaudio
    from beat_this.inference import split_piece, aggregate_prediction
    from inference.feature_extractor import AdvancedBeatFeatureExtractor
    from inference.dbn_postprocessor import DBNDownBeatTracker
    from inference.helpers import aggregate_series_to_bins, calculate_overall_bpm, compute_band_rms_frames
    from inference.resources.skey_model import KEY_MAP
    from inference.vocal_activity import FireRedFbank, split_firered_features, infer_firered_chunk, vocal_activity_probabilities

    data = np.memmap(path, dtype='<f4', mode='r')
    duration = len(data) / rate
    if rate != 48000 or not 8 <= duration <= 1200 or not np.isfinite(data).all():
        raise ValueError('Invalid analysis PCM')
    beat, (vqt, chromanet, crop), (vocal, means, inv_std) = load_models()
    mel = AdvancedBeatFeatureExtractor()
    fbank = FireRedFbank(means, inv_std)
    rs_beat = soxr.ResampleStream(rate, 22050, 1, dtype='float32')
    rs_vocal = soxr.ResampleStream(rate, 16000, 1, dtype='float32')
    mel_parts, key_parts, vocal_parts, energy_parts, centroids = [], [], [], [], []
    bands = {name: [] for name in ('low', 'low_mid', 'mid', 'high')}
    centroid = torchaudio.transforms.SpectralCentroid(sample_rate=22050, hop_length=512)

    async def extract():
        for start in range(0, len(data), rate * 10):
            last = start + rate * 10 >= len(data)
            block = np.array(data[start:start + rate * 10], dtype=np.float32)
            pcm = rs_beat.resample_chunk(block, last=last)
            voice_pcm = rs_vocal.resample_chunk(block, last=last)
            mel_parts.append(await mel.process_pcm(pcm))
            vocal_parts.append(fbank.process(voice_pcm))
            # Preserve streaming feature boundaries, as in MA's provider.
            tensor = torch.from_numpy(pcm)
            with torch.inference_mode():
                for offset in range(0, len(pcm), 22050):
                    piece = tensor[offset:offset + 22050]
                    if len(piece) >= 22050:
                        key_parts.append(crop(vqt(piece[None, None]), torch.zeros(1)).cpu())
                centroids.append(centroid(tensor).cpu().numpy())
            step = 2205
            energy_parts.append(np.array([np.sqrt(np.mean(pcm[i:i + step] ** 2)) for i in range(0, len(pcm), step)], dtype=np.float32))
            for name, values in compute_band_rms_frames(pcm, 22050, step).items():
                bands[name].append(values)
        mel_parts.append(await mel.finalize())
        vocal_parts.append(fbank.finalize())

    asyncio.run(extract())
    spect = torch.from_numpy(np.concatenate(mel_parts))
    windows, starts = split_piece(spect, 1500, border_size=6, avoid_short_end=True)
    predictions = []
    with torch.inference_mode():
        for window in windows:
            result = beat.model(window[None])
            predictions.append({k: result[k][0] for k in ('beat', 'downbeat')})
        logits, down_logits = aggregate_prediction(predictions, starts, len(spect), 1500, 6, 'keep_first', 'cpu')
        bp = torch.sigmoid(logits.float()).numpy()
        dp = torch.sigmoid(down_logits.float()).numpy()
    epsilon = 1e-5
    bp, dp = bp * (1 - epsilon) + epsilon / 2, dp * (1 - epsilon) + epsilon / 2
    tracked, meter = DBNDownBeatTracker(beats_per_bar=[3, 4], min_bpm=55, max_bpm=215, fps=50)(np.column_stack([np.maximum(bp - dp, epsilon / 2), dp]))
    beats = tracked[:, 0]
    downbeats = tracked[tracked[:, 1] == 1, 0]
    key, mode = None, None
    if key_parts:
        features = torch.cat(key_parts, dim=-1)
        if features.shape[-1] >= 128:
            with torch.inference_mode():
                label = KEY_MAP[int(chromanet(features).argmax(dim=-1).item())]
                key, mode = label.split()
                mode = mode.lower()
    voice_features = np.concatenate(vocal_parts)
    voice_predictions = []
    for chunk, offset, length in split_firered_features(voice_features):
        voice_predictions.append(infer_firered_chunk(vocal, chunk)[offset:offset + length])
    probabilities = vocal_activity_probabilities(np.concatenate(voice_predictions), duration)
    energy = aggregate_series_to_bins(np.concatenate(energy_parts), 1800, power=True)
    peak = float(energy.max())
    scale = peak if peak > 0 else 1.0
    spectral = aggregate_series_to_bins(np.concatenate(centroids), 1800)
    spectral[energy / scale < .01] = 0
    output = dict(version=VERSION, duration=duration,
                  bpm=calculate_overall_bpm(beats) if len(beats) >= 2 else None,
                  beats=beats.tolist(), downbeats=downbeats.tolist(), beats_per_bar=meter or None,
                  key=key, mode=mode, rms_energy=(energy / scale).tolist(),
                  spectral_centroid=spectral.tolist(),
                  vocal_activity=aggregate_series_to_bins(probabilities, 1800).tolist())
    for name, parts in bands.items():
        output['band_rms_' + name] = (aggregate_series_to_bins(np.concatenate(parts), 1800, power=True) / scale).tolist()
    return output


def plan(outgoing: Path, incoming: Path, buffer: float):
    from ma.planner import SmartCrossFadePlanner
    from ma.models import SmartFadeNotApplicable
    a = SimpleNamespace(**json.loads(outgoing.read_text()))
    b = SimpleNamespace(**json.loads(incoming.read_text()))
    if a.version != VERSION or b.version != VERSION:
        raise ValueError('Unsupported analysis version')
    try:
        result = SmartCrossFadePlanner(logging.getLogger('smart_fades')).plan(a, b, buffer)
    except (SmartFadeNotApplicable, ValueError):
        return {'version': VERSION, 'applicable': False}
    return {'version': VERSION, 'applicable': True, **asdict(result)}


def main():
    parser = argparse.ArgumentParser()
    sub = parser.add_subparsers(dest='command', required=True)
    a = sub.add_parser('analyze')
    a.add_argument('pcm', type=Path)
    a.add_argument('output', type=Path)
    a.add_argument('--rate', type=int, default=48000)
    p = sub.add_parser('plan')
    p.add_argument('outgoing', type=Path)
    p.add_argument('incoming', type=Path)
    p.add_argument('buffer', type=float)
    p.add_argument('output', type=Path)
    sub.add_parser('check')
    args = parser.parse_args()
    with contextlib.redirect_stdout(sys.stderr):
        if args.command == 'analyze':
            result = analyze(args.pcm, args.rate)
        elif args.command == 'plan':
            result = plan(args.outgoing, args.incoming, args.buffer)
        else:
            load_models()
            result = {'version': VERSION, 'models': ['beat-this-small0', 'S-KEY', 'FireRed-AED']}
    encoded = json.dumps(result, allow_nan=False, default=lambda v: v.value if isinstance(v, Enum) else v, separators=(',', ':'))
    if hasattr(args, 'output'):
        args.output.write_text(encoded)
    else:
        print(encoded)


if __name__ == '__main__':
    # Give audio/UI work priority; Torch's internal parallelism is separately bounded.
    os.nice(10)
    main()
