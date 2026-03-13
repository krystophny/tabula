# Model Download Policy

> **Legal notice:** Tabura is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

This policy defines what Tabura downloads, from where, under which license, and
how user consent is handled.

## Consent Tiers

### Tier 1: Silent

Permissive components (MIT/BSD/Apache/OFL) may be bundled or auto-downloaded
without an explicit prompt.

Examples:
- Tabura binary (MIT)
- ONNX Runtime Web (MIT)
- openWakeWord models (Apache-2.0)
- DistilBERT model artifacts (Apache-2.0)
- llama.cpp binaries (MIT)

### Tier 2: Notice + Opt-Out

Components with additional distribution obligations or per-model terms require a
clear notice before installation/download. Install proceeds unless the user opts
out.

Examples:
- Piper TTS runtime (GPL, deployed as local HTTP sidecar)
- Piper voice models (per-model terms documented in model cards)
- ffmpeg (GPL/LGPL, package dependent)
- voxtype (MIT external STT sidecar)
- Qwen3 0.6B GGUF (Apache-2.0 model download)

## Current Downloaded Components

| Component | Source | License | Tier |
|---|---|---|---|
| Piper TTS runtime | PyPI (`piper-tts`) | GPL | Tier 2 |
| Piper voice models | Hugging Face (`rhasspy/piper-voices`) | Per-model | Tier 2 |
| ffmpeg | System package manager | GPL/LGPL | Tier 2 |
| voxtype | AUR/Homebrew/source build (`peteonrails/voxtype` branch `feature/single-daemon-openai-stt-api` for source installs) | MIT | Tier 2 |

## Planned Model Downloads

| Component | Source | License | Tier |
|---|---|---|---|
| ONNX Runtime Web | npm/web artifact source | MIT | Tier 1 |
| openWakeWord models | project/model registry | Apache-2.0 | Tier 1 |
| DistilBERT intent model | model registry | Apache-2.0 | Tier 1 |
| Qwen3 0.6B GGUF | Hugging Face | Apache-2.0 | Tier 2 |

## Operator Rules

- Never statically or dynamically link Piper runtime libraries into the Go
  binary.
- Keep Piper integration on loopback HTTP.
- Display model/runtime notice text before Tier-2 downloads in setup scripts.
- Keep [`THIRD_PARTY_LICENSES.md`](../THIRD_PARTY_LICENSES.md) updated whenever
  dependencies or download behavior change.
