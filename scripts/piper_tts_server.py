"""Piper TTS server with OpenAI-compatible /v1/audio/speech endpoint.

Loads one Piper ONNX model per language and routes by the ``voice`` field.
Send voice="en" for English, voice="de" for German.  Unrecognised values
fall back to English.

Start::

    uvicorn piper_tts_server:app --host 127.0.0.1 --port 8424
"""

import io
import os
import wave

from fastapi import FastAPI, Request
from fastapi.responses import Response

MODEL_DIR = os.environ.get(
    "PIPER_MODEL_DIR",
    os.path.join(os.path.dirname(__file__), "..", ".local", "share", "tabura-piper-tts", "models"),
)

MODELS = {
    "en": os.path.join(MODEL_DIR, "en_GB-alan-medium.onnx"),
    "de": os.path.join(MODEL_DIR, "de_DE-karlsson-low.onnx"),
}

app = FastAPI()
_voices: dict = {}


def _load_voice(lang: str):
    from piper import PiperVoice

    path = MODELS.get(lang)
    if not path or not os.path.isfile(path):
        raise FileNotFoundError(f"model not found: {path}")
    return PiperVoice.load(path)


def _get_voice(lang: str):
    if lang not in _voices:
        _voices[lang] = _load_voice(lang)
    return _voices[lang]


@app.on_event("startup")
def preload():
    for lang, path in MODELS.items():
        if os.path.isfile(path):
            print(f"loading piper voice: {lang} -> {path}")
            _voices[lang] = _load_voice(lang)
        else:
            print(f"skipping piper voice (not found): {lang} -> {path}")


@app.get("/health")
def health():
    return {"status": "ok", "loaded_voices": list(_voices.keys())}


@app.post("/v1/audio/speech")
async def speech(request: Request):
    body = await request.json()
    text = str(body.get("input", "")).strip()
    if not text:
        return Response(content=b"", status_code=400)

    voice_key = str(body.get("voice", "en")).strip().lower()
    lang = "de" if voice_key in ("de", "de_de", "german") else "en"

    voice = _get_voice(lang)

    pcm_chunks = []
    for chunk in voice.synthesize(text):
        pcm_chunks.append(chunk.audio_int16_bytes)

    buf = io.BytesIO()
    with wave.open(buf, "wb") as wav:
        wav.setnchannels(1)
        wav.setsampwidth(2)
        wav.setframerate(voice.config.sample_rate)
        wav.writeframes(b"".join(pcm_chunks))

    return Response(content=buf.getvalue(), media_type="audio/wav")
