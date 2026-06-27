#!/usr/bin/env python3
"""Export the LogisticRegression weights from the PixelWise sklearn pipeline
to a plain JSON file that the Go re-implementation loads at startup.

The Go service never loads the .pkl. It recomputes inference from these
weights:  binarize -> logits = X·Wᵀ + b -> softmax -> argmax.

Crucially, this script also verifies *on the Python side* that a manual
softmax(X·Wᵀ + b) reproduces pipeline.predict_proba exactly. That single
check confirms the model is multinomial (one softmax over 9 logits) rather
than one-vs-rest (per-class sigmoid, then renormalise). If sklearn's default
ever changes, this assertion fails here -- loudly, in Python -- instead of
silently producing wrong probabilities in Go.
"""
import argparse
import json
import sys

import numpy as np
import joblib


def softmax(z: np.ndarray) -> np.ndarray:
    # Subtract the row max for numerical stability (sklearn does the same).
    z = z - z.max(axis=1, keepdims=True)
    e = np.exp(z)
    return e / e.sum(axis=1, keepdims=True)


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--model", default="models/digit_classifier_v1.pkl",
                    help="path to the sklearn pipeline .pkl")
    ap.add_argument("--out", default="models/weights.json",
                    help="where to write the exported weights")
    args = ap.parse_args()

    pipe = joblib.load(args.model)

    # The pipeline is Binarizer(0.5) -> LogisticRegression, named per MODELCARD.
    clf = pipe.named_steps["clf"]
    coef = np.asarray(clf.coef_, dtype=np.float64)          # (9, 784)
    intercept = np.asarray(clf.intercept_, dtype=np.float64)  # (9,)
    classes = [str(c) for c in clf.classes_]

    if coef.ndim != 2 or coef.shape[0] != len(classes) or coef.shape[1] != 784:
        sys.exit(f"unexpected coef shape {coef.shape} for {len(classes)} classes")

    # --- self-check: manual softmax must equal predict_proba ---
    # Random binary inputs {0,1}; the pipeline's Binarizer(0.5) is idempotent
    # on these, so predict_proba == clf's softmax over X·Wᵀ + b iff multinomial.
    rng = np.random.default_rng(0)
    X = (rng.random((256, 784)) > 0.5).astype(np.float64)
    manual = softmax(X @ coef.T + intercept)
    proba = pipe.predict_proba(X)
    max_err = float(np.abs(manual - proba).max())
    if max_err > 1e-9:
        sys.exit(f"softmax reconstruction != predict_proba (max err {max_err:.2e}); "
                 "model may not be multinomial -- investigate before porting to Go")

    out = {
        "classes": classes,           # index order == proba/argmax order
        "n_features": int(coef.shape[1]),
        "input_threshold": 128,       # raw 0-255 pixels are binarized at >128
        "coef": coef.tolist(),        # row k holds the weights for classes[k]
        "intercept": intercept.tolist(),
    }
    with open(args.out, "w") as f:
        json.dump(out, f)
    print(f"wrote {args.out}: {len(classes)} classes, {coef.shape[1]} features, "
          f"reconstruction verified (max err {max_err:.2e})")



main()
