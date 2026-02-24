package eou

import "strings"

const (
	ReasonHighConfidenceEnd   = "high_confidence_end"
	ReasonLowConfidenceCont   = "low_confidence_continue"
	ReasonHardSilenceCommit   = "hard_silence_commit"
	ReasonMaxDurationCommit   = "max_duration_commit"
	ReasonEmptyTranscriptCont = "empty_transcript_continue"
	ReasonFallbackVADCommit   = "fallback_vad_commit"
	ReasonFallbackVADContinue = "fallback_vad_continue"
	ReasonNoSemanticScore     = "no_semantic_score_continue"
	DefaultCandidateSilenceMs = 900
	DefaultHardSilenceMs      = 2500
	DefaultMaxRecordingMs     = 20000
)

type DecisionInput struct {
	Transcript         string
	SilenceMS          int
	ElapsedMS          int
	PEnd               float64
	HasSemanticScore   bool
	FallbackToVAD      bool
	CommitThreshold    float64
	CandidateSilenceMS int
	HardSilenceMS      int
	MaxRecordingMS     int
}

type Decision struct {
	ShouldCommit bool
	Reason       string
}

func Decide(input DecisionInput) Decision {
	transcript := strings.TrimSpace(input.Transcript)
	if transcript == "" {
		return Decision{ShouldCommit: false, Reason: ReasonEmptyTranscriptCont}
	}

	candidateSilence := input.CandidateSilenceMS
	if candidateSilence <= 0 {
		candidateSilence = DefaultCandidateSilenceMs
	}
	hardSilence := input.HardSilenceMS
	if hardSilence <= 0 {
		hardSilence = DefaultHardSilenceMs
	}
	maxRecording := input.MaxRecordingMS
	if maxRecording <= 0 {
		maxRecording = DefaultMaxRecordingMs
	}
	threshold := input.CommitThreshold
	if threshold <= 0 || threshold > 1 {
		threshold = DefaultCommitScore
	}

	if input.ElapsedMS >= maxRecording {
		return Decision{ShouldCommit: true, Reason: ReasonMaxDurationCommit}
	}
	if input.SilenceMS >= hardSilence {
		return Decision{ShouldCommit: true, Reason: ReasonHardSilenceCommit}
	}

	if input.FallbackToVAD {
		if input.SilenceMS >= candidateSilence {
			return Decision{ShouldCommit: true, Reason: ReasonFallbackVADCommit}
		}
		return Decision{ShouldCommit: false, Reason: ReasonFallbackVADContinue}
	}

	if !input.HasSemanticScore {
		return Decision{ShouldCommit: false, Reason: ReasonNoSemanticScore}
	}
	if input.PEnd >= threshold {
		return Decision{ShouldCommit: true, Reason: ReasonHighConfidenceEnd}
	}
	return Decision{ShouldCommit: false, Reason: ReasonLowConfidenceCont}
}
