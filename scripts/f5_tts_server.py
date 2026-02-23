"""F5-TTS server with OpenAI-compatible /v1/audio/speech endpoint.

Uses F5-TTS Fast mode (EPSS, 7 NFE steps) for low-latency, high-quality
voice-cloned synthesis.  English uses the base F5-TTS model, German uses
aihpi/F5-TTS-German.  Both share a single reference WAV for voice cloning.

Send voice="en" for English, voice="de" for German.

Start::

    uvicorn f5_tts_server:app --host 127.0.0.1 --port 8424
"""

import io
import os
import time
import wave

import torch
from fastapi import FastAPI, Request
from fastapi.responses import Response

REF_WAV = os.environ.get(
    "F5_TTS_REF_WAV",
    os.path.expanduser("~/.local/share/tabura-tts/reference.wav"),
)
REF_TEXT = os.environ.get("F5_TTS_REF_TEXT", "")

SAMPLE_RATE = 24000
NFE_STEPS = 7

app = FastAPI()

_models: dict = {}
_ref_wav_path: str = ""
_ref_text: str = ""


def _load_model(lang: str):
    from f5_tts.api import F5TTS

    if lang == "de":
        return F5TTS(model_type="F5-TTS", ckpt_file="aihpi/F5-TTS-German")
    return F5TTS(model_type="F5-TTS")


@app.on_event("startup")
def preload():
    global _ref_wav_path, _ref_text

    _ref_wav_path = REF_WAV
    if not os.path.isfile(_ref_wav_path):
        print(f"WARNING: reference WAV not found: {_ref_wav_path}")

    _ref_text = REF_TEXT

    for lang in ("en", "de"):
        print(f"loading F5-TTS model: {lang} ...")
        t0 = time.monotonic()
        _models[lang] = _load_model(lang)
        dt = time.monotonic() - t0
        print(f"  {lang} loaded in {dt:.1f}s")

    device = "cuda" if torch.cuda.is_available() else "cpu"
    print(f"F5-TTS ready on {device}, NFE steps={NFE_STEPS}")


@app.get("/health")
def health():
    device = "cuda" if torch.cuda.is_available() else "cpu"
    return {
        "status": "ok",
        "backend": "f5-tts",
        "device": device,
        "nfe_steps": NFE_STEPS,
        "loaded_models": list(_models.keys()),
        "ref_wav": _ref_wav_path,
    }


@app.post("/v1/audio/speech")
async def speech(request: Request):
    body = await request.json()
    text = str(body.get("input", "")).strip()
    if not text:
        return Response(content=b"", status_code=400)

    voice_key = str(body.get("voice", "en")).strip().lower()
    lang = "de" if voice_key in ("de", "de_de", "german") else "en"

    model = _models.get(lang)
    if model is None:
        return Response(
            content=f"model not loaded: {lang}".encode(),
            status_code=503,
        )

    if not os.path.isfile(_ref_wav_path):
        return Response(
            content=f"reference WAV not found: {_ref_wav_path}".encode(),
            status_code=503,
        )

    t0 = time.monotonic()
    with torch.inference_mode():
        wav_audio, sr, _ = model.infer(
            ref_file=_ref_wav_path,
            ref_text=_ref_text,
            gen_text=text,
            nfe_step=NFE_STEPS,
        )
    dt = time.monotonic() - t0

    samples = wav_audio.squeeze()
    if samples.is_floating_point():
        samples = (samples * 32767).clamp(-32768, 32767).to(torch.int16)
    pcm = samples.cpu().numpy().tobytes()

    buf = io.BytesIO()
    with wave.open(buf, "wb") as wf:
        wf.setnchannels(1)
        wf.setsampwidth(2)
        wf.setframerate(sr)
        wf.writeframes(pcm)

    duration = len(pcm) / (2 * sr)
    print(f"TTS [{lang}] {duration:.2f}s audio in {dt:.3f}s (RTF {dt/duration:.3f})")

    return Response(content=buf.getvalue(), media_type="audio/wav")
