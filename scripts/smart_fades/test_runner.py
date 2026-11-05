"""Behavior tests for the standalone MA planner contract; no audio is played."""
import json
from pathlib import Path
import tempfile
import unittest

from runner import plan


def track(bpm=120, meter=4):
    duration = 120
    beat = 60 / bpm
    beats = [i * beat for i in range(int(duration / beat))]
    # Strong low and mid-band energy, with a measured instrumental ending.
    energy = [1.0 if i < 1650 else max(.1, (1800 - i) / 150) for i in range(1800)]
    return dict(version=1, duration=duration, bpm=bpm, beats=beats,
                downbeats=beats[::meter], beats_per_bar=meter, key='C', mode='major',
                rms_energy=energy, spectral_centroid=[2000.] * 1800,
                vocal_activity=[0.] * 1800,
                band_rms_low=[x * .5 for x in energy],
                band_rms_low_mid=[x * .35 for x in energy],
                band_rms_mid=[x * .6 for x in energy],
                band_rms_high=[x * .15 for x in energy])


class PlannerBehavior(unittest.TestCase):
    def run_pair(self, a, b):
        with tempfile.TemporaryDirectory() as d:
            out, incoming = Path(d) / 'a.json', Path(d) / 'b.json'
            out.write_text(json.dumps(a))
            incoming.write_text(json.dumps(b))
            return plan(out, incoming, 45)

    # Compatible tempos should produce a pitch-preserving tempo schedule, not a fixed fade.
    def test_compatible_beats_get_tempo_plan(self):
        result = self.run_pair(track(), track(124))
        self.assertTrue(result['applicable'])
        steps = result['tempo_plan']['steps']
        self.assertTrue(steps)
        self.assertAlmostEqual(steps[-1][1], 124 / 120, places=5)
        self.assertGreater(result['crossfade_duration'], 1)

    # Different meters cannot be forced into a shared bar grid.
    def test_different_meters_do_not_beatmatch(self):
        result = self.run_pair(track(meter=3), track(meter=4))
        self.assertTrue(result['applicable'])
        self.assertFalse(result['tempo_plan']['steps'])

    # Missing rhythmic evidence must preserve a natural boundary instead of inventing a tempo.
    def test_missing_bpm_does_not_invent_beats(self):
        a = track()
        a['bpm'] = None
        self.assertFalse(self.run_pair(a, track())['applicable'])


if __name__ == '__main__':
    unittest.main()
