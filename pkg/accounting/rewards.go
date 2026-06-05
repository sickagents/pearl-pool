package accounting

import (
	"math"
)

type RewardCalculator struct {
	mode           string
	pplnsWindow    int
	poolFee        float64
	confirmationDepth int
}

func NewRewardCalculator(mode string, pplnsWindow int, poolFee float64, confirmDepth int) *RewardCalculator {
	return &RewardCalculator{
		mode:              mode,
		pplnsWindow:       pplnsWindow,
		poolFee:           poolFee,
		confirmationDepth: confirmDepth,
	}
}

type ShareWindow struct {
	Shares []ShareRecord
}

type ShareRecord struct {
	Address    string
	Difficulty float64
	Height     int64
}

type Reward struct {
	Address string
	Amount  int64
}

func (rc *RewardCalculator) CalculatePPLNS(blockReward int64, shares []ShareRecord) []Reward {
	if len(shares) == 0 {
		return nil
	}
	
	window := shares
	if len(shares) > rc.pplnsWindow {
		window = shares[len(shares)-rc.pplnsWindow:]
	}
	
	totalDifficulty := 0.0
	minerShares := make(map[string]float64)
	
	for _, share := range window {
		totalDifficulty += share.Difficulty
		minerShares[share.Address] += share.Difficulty
	}
	
	if totalDifficulty == 0 {
		return nil
	}
	
	feeAmount := int64(math.Floor(float64(blockReward) * rc.poolFee / 100.0))
	distributableReward := blockReward - feeAmount
	
	var rewards []Reward
	for address, difficulty := range minerShares {
		share := difficulty / totalDifficulty
		amount := int64(math.Floor(float64(distributableReward) * share))
		if amount > 0 {
			rewards = append(rewards, Reward{
				Address: address,
				Amount:  amount,
			})
		}
	}
	
	return rewards
}

func (rc *RewardCalculator) CalculatePROP(blockReward int64, shares []ShareRecord, roundStart int64) []Reward {
	var roundShares []ShareRecord
	for _, share := range shares {
		if share.Height >= roundStart {
			roundShares = append(roundShares, share)
		}
	}
	
	if len(roundShares) == 0 {
		return nil
	}
	
	totalDifficulty := 0.0
	minerShares := make(map[string]float64)
	
	for _, share := range roundShares {
		totalDifficulty += share.Difficulty
		minerShares[share.Address] += share.Difficulty
	}
	
	if totalDifficulty == 0 {
		return nil
	}
	
	feeAmount := int64(math.Floor(float64(blockReward) * rc.poolFee / 100.0))
	distributableReward := blockReward - feeAmount
	
	var rewards []Reward
	for address, difficulty := range minerShares {
		share := difficulty / totalDifficulty
		amount := int64(math.Floor(float64(distributableReward) * share))
		if amount > 0 {
			rewards = append(rewards, Reward{
				Address: address,
				Amount:  amount,
			})
		}
	}
	
	return rewards
}

func (rc *RewardCalculator) CalculateSolo(blockReward int64, finderAddress string) []Reward {
	feeAmount := int64(math.Floor(float64(blockReward) * rc.poolFee / 100.0))
	minerReward := blockReward - feeAmount
	
	return []Reward{
		{
			Address: finderAddress,
			Amount:  minerReward,
		},
	}
}
