"""RAGAS-style faithfulness and answer relevancy stubs for RAG pipeline evaluation.

These are lightweight local approximations of RAGAS metrics that do not require
an LLM judge call. Use the real ragas library for production scoring; these stubs
provide fast, offline signal during development and CI.

References:
  - RAGAS paper: https://arxiv.org/abs/2309.15217
  - ragas library: https://github.com/explodinggradients/ragas
"""

from __future__ import annotations

import re
from dataclasses import dataclass


def _sentences(text: str) -> list[str]:
    """Split text into sentences on . ! ? boundaries."""
    parts = re.split(r"(?<=[.!?])\s+", text.strip())
    return [p.strip() for p in parts if p.strip()]


def _token_set(text: str) -> set[str]:
    tokens = re.sub(r"[^\w\s]", " ", text.lower(), flags=re.UNICODE).split()
    return set(tokens)


def _jaccard(a: set[str], b: set[str]) -> float:
    if not a and not b:
        return 1.0
    intersection = len(a & b)
    union = len(a | b)
    return intersection / union if union > 0 else 0.0


@dataclass
class FaithfulnessResult:
    """
    Faithfulness: fraction of answer sentences that are entailed by the retrieved context.

    Score of 1.0 means every claim in the answer can be traced to the context.
    Score of 0.0 means no overlap. Hallucinated answers score low.
    """

    score: float  # [0, 1]
    supported_sentences: int
    total_sentences: int
    sentence_scores: list[float]

    @property
    def verdict(self) -> str:
        if self.score >= 0.8:
            return "faithful"
        if self.score >= 0.5:
            return "partial"
        return "hallucinated"


@dataclass
class RelevancyResult:
    """
    Answer relevancy: how well the answer addresses the question.

    Approximated as token-level Jaccard similarity between question and answer,
    with a small boost for length ratio. Not a substitute for LLM-based scoring.
    """

    score: float  # [0, 1]
    question_tokens: int
    answer_tokens: int

    @property
    def verdict(self) -> str:
        if self.score >= 0.5:
            return "relevant"
        if self.score >= 0.2:
            return "partial"
        return "irrelevant"


def faithfulness(answer: str, context: str, threshold: float = 0.15) -> FaithfulnessResult:
    """
    Compute approximate faithfulness of an answer given retrieved context.

    Implementation: for each sentence in the answer, compute Jaccard similarity
    against the full context token set. A sentence is considered "supported" if
    its Jaccard similarity exceeds `threshold`.

    Args:
        answer: The generated answer text.
        context: Concatenated retrieved passage(s).
        threshold: Minimum Jaccard overlap to consider a sentence supported.

    Returns:
        FaithfulnessResult with score in [0, 1].
    """
    if not answer.strip():
        return FaithfulnessResult(
            score=0.0,
            supported_sentences=0,
            total_sentences=0,
            sentence_scores=[],
        )

    context_tokens = _token_set(context)
    sentences = _sentences(answer)
    sentence_scores: list[float] = []

    for sent in sentences:
        sent_tokens = _token_set(sent)
        score = _jaccard(sent_tokens, context_tokens)
        sentence_scores.append(round(score, 4))

    supported = sum(1 for s in sentence_scores if s >= threshold)
    total = len(sentences)
    overall = supported / total if total > 0 else 0.0

    return FaithfulnessResult(
        score=round(overall, 4),
        supported_sentences=supported,
        total_sentences=total,
        sentence_scores=sentence_scores,
    )


def answer_relevancy(question: str, answer: str) -> RelevancyResult:
    """
    Compute approximate answer relevancy to a question.

    Implementation: Jaccard similarity between question tokens and answer tokens,
    adjusted by answer length relative to question length (penalises very short answers).

    Args:
        question: The user question.
        answer: The generated answer.

    Returns:
        RelevancyResult with score in [0, 1].
    """
    q_tokens = _token_set(question)
    a_tokens = _token_set(answer)

    base_score = _jaccard(q_tokens, a_tokens)

    # Length penalty: if answer is much shorter than question, reduce score.
    q_len = len(q_tokens)
    a_len = len(a_tokens)
    length_ratio = min(a_len / q_len, 1.0) if q_len > 0 else 1.0
    adjusted = base_score * (0.5 + 0.5 * length_ratio)

    return RelevancyResult(
        score=round(adjusted, 4),
        question_tokens=q_len,
        answer_tokens=a_len,
    )


@dataclass
class RAGEvalResult:
    faithfulness: FaithfulnessResult
    relevancy: RelevancyResult

    @property
    def combined_score(self) -> float:
        """Harmonic mean of faithfulness and relevancy scores."""
        f = self.faithfulness.score
        r = self.relevancy.score
        if f + r == 0:
            return 0.0
        return round(2 * f * r / (f + r), 4)


def evaluate_rag(question: str, answer: str, context: str) -> RAGEvalResult:
    """
    Run both faithfulness and relevancy metrics for a single RAG response.

    Args:
        question: User question.
        answer: Generated answer from the RAG pipeline.
        context: Retrieved context passed to the generator.

    Returns:
        RAGEvalResult with both metric results and combined score.
    """
    return RAGEvalResult(
        faithfulness=faithfulness(answer, context),
        relevancy=answer_relevancy(question, answer),
    )


def evaluate_rag_dataset(
    cases: list[dict[str, str]],
) -> dict[str, float]:
    """
    Aggregate RAG metrics across a dataset.

    Each case dict must have keys: 'question', 'answer', 'context'.

    Returns:
        Dict with 'avg_faithfulness', 'avg_relevancy', 'avg_combined'.
    """
    results = [
        evaluate_rag(c["question"], c["answer"], c["context"]) for c in cases
    ]
    if not results:
        return {"avg_faithfulness": 0.0, "avg_relevancy": 0.0, "avg_combined": 0.0}

    n = len(results)
    return {
        "avg_faithfulness": round(sum(r.faithfulness.score for r in results) / n, 4),
        "avg_relevancy": round(sum(r.relevancy.score for r in results) / n, 4),
        "avg_combined": round(sum(r.combined_score for r in results) / n, 4),
    }
