"""
This file tests the Go re-implementation against a set of inputs
to insure the models prediction is the same as the Python version
"""

import json, numpy as np
import sys
from pathlib import Path

# Fixes the import of app.classifier
sys.path.insert(0, str(Path(__file__).parent.parent))

from app.classifier import classify_batch


cases = []

# Edge cases
cases.append(np.zeros((28, 28), dtype=np.uint8))     # empty image
cases.append(np.full((28, 28), 255, dtype=np.uint8)) # all white
cases.append((np.eye(28) * 255).astype(np.uint8))    # diagonal

# Random cases, seeded for reproducibility
rng = np.random.default_rng(42)
for _ in range(23):
    cases.append((rng.integers(0, 256, (28, 28), dtype=np.uint8)))

golden = []
for px in cases:
    result = classify_batch(px[np.newaxis])[0]
    golden.append({
        "pixels": px.tolist(),              # the input to the model
        "prediction": result["prediction"], # Output of the python model
        "confidence": result["confidence"], # Output of the python model
        "scores": result["scores"],
    })

with open("testdata/golden.json", "w") as f:
    json.dump(golden, f, indent=2)

print(f"wrote {len(golden)} cases")