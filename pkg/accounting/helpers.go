package accounting

// GetPPLNSWindow returns the PPLNS window size
func (rc *RewardCalculator) GetPPLNSWindow() int {
	return rc.pplnsWindow
}
