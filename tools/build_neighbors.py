#!/usr/bin/env python3
"""Build the nearest-neighbor table used by horsepaste "hard mode".

Hard mode picks a random assassin word and fills the board with words that are
semantically closest to it. To keep the Go server free of any ML runtime, we
precompute a static table offline here and commit the result.

The table maps each default word to the other default words ranked by cosine
similarity (closest first), restricted to the default word list.

Usage:
    pip install gensim
    python tools/build_neighbors.py

This downloads a pretrained GloVe model (~130MB) the first time it runs, so it
needs network access. The output, assets/neighbors.json, is committed to the
repo; the server only reads that file.
"""

import json
import os

import gensim.downloader as api

# Resolve paths relative to the repo root so the script works from anywhere.
ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
WORDS_PATH = os.path.join(ROOT, "assets", "original.txt")
OUT_PATH = os.path.join(ROOT, "assets", "neighbors.json")

# Pretrained vectors. glove-wiki-gigaword-100 is a good size/quality tradeoff.
MODEL_NAME = "glove-wiki-gigaword-100"

# Keep at most this many neighbors per word; the board needs 24 and we sample
# loosely from roughly the top half, so this leaves comfortable headroom.
TOP_N = 60


def load_words(path):
    words = []
    with open(path) as f:
        for line in f:
            w = line.strip()
            if w:
                words.append(w.upper())
    return words


def main():
    words = load_words(WORDS_PATH)
    print(f"Loaded {len(words)} words from {WORDS_PATH}")

    print(f"Loading model {MODEL_NAME} (downloads on first run)...")
    model = api.load(MODEL_NAME)

    # Map the embedding-vocabulary form (lowercase) back to the canonical
    # uppercase board form, skipping words the model doesn't know.
    in_vocab = []
    missing = []
    for w in words:
        if w.lower() in model.key_to_index:
            in_vocab.append(w)
        else:
            missing.append(w)
    if missing:
        print(f"Skipping {len(missing)} words missing from vocab: "
              f"{', '.join(sorted(missing))}")

    vocab_set = {w.lower() for w in in_vocab}
    upper = {w.lower(): w for w in in_vocab}

    neighbors = {}
    for w in in_vocab:
        # Ask the model for many candidates, then keep only those that are also
        # on our board and take the closest TOP_N.
        ranked = model.most_similar(w.lower(), topn=len(model.key_to_index))
        ns = []
        for cand, _score in ranked:
            if cand in vocab_set and cand != w.lower():
                ns.append(upper[cand])
            if len(ns) >= TOP_N:
                break
        neighbors[w] = ns

    with open(OUT_PATH, "w") as f:
        json.dump(neighbors, f, indent=0, sort_keys=True)
    print(f"Wrote {len(neighbors)} entries to {OUT_PATH}")


if __name__ == "__main__":
    main()
