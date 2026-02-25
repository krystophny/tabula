from __future__ import annotations

import argparse
import json
import shutil
import subprocess
import sys
from pathlib import Path

from datasets import Dataset
from transformers import (
    AutoModelForSequenceClassification,
    AutoTokenizer,
    DataCollatorWithPadding,
    Trainer,
    TrainingArguments,
)


def load_dataset(path: Path) -> tuple[Dataset, list[str]]:
    records = json.loads(path.read_text(encoding="utf-8"))
    texts: list[str] = []
    intents: list[str] = []
    for record in records:
        text = str(record.get("text", "")).strip()
        intent = str(record.get("intent", "")).strip()
        if not text or not intent:
            continue
        texts.append(text)
        intents.append(intent)
    if not texts:
        raise RuntimeError(f"dataset has no training rows: {path}")

    labels = sorted(set(intents))
    label_to_id = {label: idx for idx, label in enumerate(labels)}
    encoded_labels = [label_to_id[intent] for intent in intents]
    dataset = Dataset.from_dict({"text": texts, "label": encoded_labels})
    return dataset, labels


def export_onnx(model_dir: Path) -> None:
    onnx_out = model_dir / "onnx"
    onnx_out.mkdir(parents=True, exist_ok=True)
    cmd = [
        sys.executable,
        "-m",
        "optimum.exporters.onnx",
        "--model",
        str(model_dir),
        "--task",
        "text-classification",
        str(onnx_out),
    ]
    subprocess.check_call(cmd)

    model_candidates = [
        onnx_out / "model.onnx",
        onnx_out / "model_quantized.onnx",
        onnx_out / "onnx" / "model.onnx",
    ]
    for candidate in model_candidates:
        if candidate.is_file():
            shutil.copyfile(candidate, model_dir / "model.onnx")
            return
    raise RuntimeError(f"ONNX export did not produce model file under {onnx_out}")


def main() -> None:
    parser = argparse.ArgumentParser(description="Train and export Tabura intent classifier")
    parser.add_argument("--dataset", type=Path, default=Path(__file__).parent / "intents.json")
    parser.add_argument("--model-id", default="distilbert-base-uncased")
    parser.add_argument("--output", type=Path, default=Path(__file__).parent / "model")
    parser.add_argument("--epochs", type=float, default=4.0)
    parser.add_argument("--batch-size", type=int, default=16)
    args = parser.parse_args()

    dataset, labels = load_dataset(args.dataset)
    split = dataset.train_test_split(test_size=0.2, seed=7)

    tokenizer = AutoTokenizer.from_pretrained(args.model_id)

    def tokenize(batch: dict) -> dict:
        return tokenizer(batch["text"], truncation=True, padding=False, max_length=64)

    tokenized = split.map(tokenize, batched=True)

    output_dir = args.output.resolve()
    output_dir.mkdir(parents=True, exist_ok=True)

    model = AutoModelForSequenceClassification.from_pretrained(
        args.model_id,
        num_labels=len(labels),
        id2label={idx: label for idx, label in enumerate(labels)},
        label2id={label: idx for idx, label in enumerate(labels)},
    )

    training_args = TrainingArguments(
        output_dir=str(output_dir / "checkpoints"),
        learning_rate=2e-5,
        per_device_train_batch_size=args.batch_size,
        per_device_eval_batch_size=args.batch_size,
        num_train_epochs=args.epochs,
        evaluation_strategy="epoch",
        save_strategy="no",
        logging_strategy="epoch",
        report_to=[],
        seed=7,
    )

    trainer = Trainer(
        model=model,
        args=training_args,
        train_dataset=tokenized["train"],
        eval_dataset=tokenized["test"],
        tokenizer=tokenizer,
        data_collator=DataCollatorWithPadding(tokenizer=tokenizer),
    )
    trainer.train()
    metrics = trainer.evaluate()
    print(json.dumps({"eval": metrics}, indent=2))

    trainer.save_model(str(output_dir))
    tokenizer.save_pretrained(str(output_dir / "tokenizer"))
    (output_dir / "labels.json").write_text(
        json.dumps({idx: label for idx, label in enumerate(labels)}, indent=2),
        encoding="utf-8",
    )

    export_onnx(output_dir)
    print(f"model exported to {output_dir}")


if __name__ == "__main__":
    main()
