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
SIM_PATH = os.path.join(ROOT, "assets", "similarity.json")
DISSIMILAR_PATH = os.path.join(ROOT, "assets", "dissimilar_boards.json")

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

    # Build pairwise cosine similarity table for the graph view.
    print("Computing pairwise similarity scores...")
    similarity = {}
    for i, w1 in enumerate(in_vocab):
        row = {}
        for w2 in in_vocab:
            if w1 == w2:
                continue
            score = float(model.similarity(w1.lower(), w2.lower()))
            row[w2] = round(score, 4)
        similarity[w1] = row
    with open(SIM_PATH, "w") as f:
        json.dump(similarity, f, indent=0, sort_keys=True)
    print(f"Wrote {len(similarity)} entries to {SIM_PATH}")

    print("Computing dissimilar boards...")
    boards = build_dissimilar_boards(in_vocab, similarity)
    with open(DISSIMILAR_PATH, "w") as f:
        json.dump(boards, f, indent=0)
    print(f"Wrote {len(boards)} dissimilar boards to {DISSIMILAR_PATH}")


def build_dissimilar_boards(words, similarity, board_size=25, pool_k=3):
    """Build diverse boards using greedy farthest-point sampling.

    Each board starts from a different word, then greedily adds the word
    that is farthest from all already-chosen words (measured by 1 - cosine
    similarity). At each step we pick randomly from the top-k farthest
    candidates so boards are not all identical when they share a prefix.

    Returns a list of boards, each a list of board_size word strings.
    """
    import random as _random

    rng = _random.Random(42)
    boards = []

    for start in words:
        chosen = [start]
        remaining = set(words) - {start}

        # min_dist[w] = min distance from w to any already-chosen word
        min_dist = {}
        for w in remaining:
            sim = similarity.get(start, {}).get(w, 0.0)
            min_dist[w] = 1.0 - sim

        while len(chosen) < board_size and remaining:
            sorted_rem = sorted(remaining, key=lambda w: -min_dist[w])
            k = min(pool_k, len(sorted_rem))
            pick = sorted_rem[rng.randrange(k)]

            chosen.append(pick)
            remaining.remove(pick)

            for w in remaining:
                d = 1.0 - similarity.get(pick, {}).get(w, 0.0)
                if d < min_dist[w]:
                    min_dist[w] = d

        if len(chosen) == board_size:
            boards.append(chosen)

    return boards


if __name__ == "__main__":
    main()
