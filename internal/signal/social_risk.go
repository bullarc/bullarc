package signal

import "github.com/bullarc/bullarc"

// RiskFlagElevatedSocialAttention is the flag attached to composite signals
// when a symbol has elevated retail attention on social media.
const RiskFlagElevatedSocialAttention = "elevated_social_attention"

// ApplySocialRiskFlag attaches an elevated_social_attention risk flag to sig
// when isElevated is true, and reduces its confidence by penaltyPct percent
// (e.g. penaltyPct=10 reduces confidence by 10%). The signal direction (Type)
// is never modified. If isElevated is false the signal is returned unchanged.
func ApplySocialRiskFlag(sig bullarc.Signal, isElevated bool, penaltyPct float64) bullarc.Signal {
	if !isElevated {
		return sig
	}

	// Copy existing flags to avoid mutating shared slices.
	flags := make([]string, len(sig.RiskFlags), len(sig.RiskFlags)+1)
	copy(flags, sig.RiskFlags)
	flags = append(flags, RiskFlagElevatedSocialAttention)
	sig.RiskFlags = flags

	if penaltyPct > 0 {
		sig.Confidence = sig.Confidence * (1 - penaltyPct/100)
		if sig.Confidence < 0 {
			sig.Confidence = 0
		}
	}
	return sig
}
