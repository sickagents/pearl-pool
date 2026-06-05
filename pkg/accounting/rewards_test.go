package accounting

import (
	"testing"
)

func TestCalculatePPLNS(t *testing.T) {
	calc := NewRewardCalculator("pplns", 100, 1.0, 100)
	
	shares := []ShareRecord{
		{Address: "prl1addr1", Difficulty: 1000000, Height: 100},
		{Address: "prl1addr1", Difficulty: 1000000, Height: 101},
		{Address: "prl1addr2", Difficulty: 2000000, Height: 102},
		{Address: "prl1addr3", Difficulty: 500000, Height: 103},
	}
	
	blockReward := int64(10000000000) // 100 PEARL (satoshis)
	
	rewards := calc.CalculatePPLNS(blockReward, shares)
	
	// Total difficulty: 1M + 1M + 2M + 500K = 4.5M
	// After 1% fee: 9900000000
	// addr1: (2M / 4.5M) * 9900000000 = 4400000000
	// addr2: (2M / 4.5M) * 9900000000 = 4400000000
	// addr3: (500K / 4.5M) * 9900000000 = 1100000000
	
	if len(rewards) != 3 {
		t.Fatalf("expected 3 rewards, got %d", len(rewards))
	}
	
	// Verify addr1 (2M shares = 44.44%)
	addr1Reward := findReward(rewards, "prl1addr1")
	expectedAddr1 := int64(4400000000)
	if addr1Reward.Amount != expectedAddr1 {
		t.Errorf("addr1: expected %d, got %d", expectedAddr1, addr1Reward.Amount)
	}
	
	// Verify addr2 (2M shares = 44.44%)
	addr2Reward := findReward(rewards, "prl1addr2")
	expectedAddr2 := int64(4400000000)
	if addr2Reward.Amount != expectedAddr2 {
		t.Errorf("addr2: expected %d, got %d", expectedAddr2, addr2Reward.Amount)
	}
	
	// Verify addr3 (500K shares = 11.11%)
	addr3Reward := findReward(rewards, "prl1addr3")
	expectedAddr3 := int64(1100000000)
	if addr3Reward.Amount != expectedAddr3 {
		t.Errorf("addr3: expected %d, got %d", expectedAddr3, addr3Reward.Amount)
	}
	
	// Verify total (should equal block reward minus fee)
	total := int64(0)
	for _, r := range rewards {
		total += r.Amount
	}
	
	expectedTotal := int64(9900000000)
	if total != expectedTotal {
		t.Errorf("total: expected %d, got %d", expectedTotal, total)
	}
}

func TestCalculatePROP(t *testing.T) {
	calc := NewRewardCalculator("prop", 100, 2.0, 100)
	
	shares := []ShareRecord{
		{Address: "prl1addr1", Difficulty: 1000000, Height: 100},
		{Address: "prl1addr1", Difficulty: 1000000, Height: 101},
		{Address: "prl1addr2", Difficulty: 3000000, Height: 102},
	}
	
	blockReward := int64(10000000000) // 100 PEARL
	roundStart := int64(100)
	
	rewards := calc.CalculatePROP(blockReward, shares, roundStart)
	
	// Total difficulty: 5M
	// After 2% fee: 9800000000
	// addr1: (2M / 5M) * 9800000000 = 3920000000
	// addr2: (3M / 5M) * 9800000000 = 5880000000
	
	if len(rewards) != 2 {
		t.Fatalf("expected 2 rewards, got %d", len(rewards))
	}
	
	addr1Reward := findReward(rewards, "prl1addr1")
	expectedAddr1 := int64(3920000000)
	if addr1Reward.Amount != expectedAddr1 {
		t.Errorf("addr1: expected %d, got %d", expectedAddr1, addr1Reward.Amount)
	}
	
	addr2Reward := findReward(rewards, "prl1addr2")
	expectedAddr2 := int64(5880000000)
	if addr2Reward.Amount != expectedAddr2 {
		t.Errorf("addr2: expected %d, got %d", expectedAddr2, addr2Reward.Amount)
	}
}

func TestPPLNSWindow(t *testing.T) {
	calc := NewRewardCalculator("pplns", 2, 0.0, 100) // window = 2 shares
	
	shares := []ShareRecord{
		{Address: "prl1addr1", Difficulty: 1000000, Height: 98},
		{Address: "prl1addr1", Difficulty: 1000000, Height: 99},
		{Address: "prl1addr2", Difficulty: 1000000, Height: 100},
		{Address: "prl1addr2", Difficulty: 1000000, Height: 101},
	}
	
	blockReward := int64(10000000000)
	
	rewards := calc.CalculatePPLNS(blockReward, shares)
	
	// Only last 2 shares counted (height 100, 101)
	// Both addr2, so addr2 gets 100%
	
	if len(rewards) != 1 {
		t.Fatalf("expected 1 reward (only addr2), got %d", len(rewards))
	}
	
	addr2Reward := findReward(rewards, "prl1addr2")
	if addr2Reward.Amount != blockReward {
		t.Errorf("addr2 should get full reward: expected %d, got %d", blockReward, addr2Reward.Amount)
	}
}

func TestZeroFee(t *testing.T) {
	calc := NewRewardCalculator("pplns", 100, 0.0, 100)
	
	shares := []ShareRecord{
		{Address: "prl1addr1", Difficulty: 1000000, Height: 100},
	}
	
	blockReward := int64(10000000000)
	
	rewards := calc.CalculatePPLNS(blockReward, shares)
	
	if len(rewards) != 1 {
		t.Fatalf("expected 1 reward, got %d", len(rewards))
	}
	
	// With 0% fee, miner gets full reward
	if rewards[0].Amount != blockReward {
		t.Errorf("expected full reward %d, got %d", blockReward, rewards[0].Amount)
	}
}

func TestHighFee(t *testing.T) {
	calc := NewRewardCalculator("pplns", 100, 10.0, 100)
	
	shares := []ShareRecord{
		{Address: "prl1addr1", Difficulty: 1000000, Height: 100},
	}
	
	blockReward := int64(10000000000)
	
	rewards := calc.CalculatePPLNS(blockReward, shares)
	
	if len(rewards) != 1 {
		t.Fatalf("expected 1 reward, got %d", len(rewards))
	}
	
	// 10% fee: 9000000000
	expected := int64(9000000000)
	if rewards[0].Amount != expected {
		t.Errorf("expected %d after 10%% fee, got %d", expected, rewards[0].Amount)
	}
}

func TestEmptyShares(t *testing.T) {
	calc := NewRewardCalculator("pplns", 100, 1.0, 100)
	
	shares := []ShareRecord{}
	blockReward := int64(10000000000)
	
	rewards := calc.CalculatePPLNS(blockReward, shares)
	
	if len(rewards) != 0 {
		t.Fatalf("expected 0 rewards for empty shares, got %d", len(rewards))
	}
}

func TestMultipleMinersPROP(t *testing.T) {
	calc := NewRewardCalculator("prop", 100, 1.5, 100)
	
	shares := []ShareRecord{
		{Address: "prl1miner1", Difficulty: 5000000, Height: 100},
		{Address: "prl1miner2", Difficulty: 3000000, Height: 101},
		{Address: "prl1miner3", Difficulty: 2000000, Height: 102},
	}
	
	blockReward := int64(10000000000)
	roundStart := int64(100)
	
	rewards := calc.CalculatePROP(blockReward, shares, roundStart)
	
	// Total: 10M difficulty
	// After 1.5% fee: 9850000000
	// miner1: 50% = 4925000000
	// miner2: 30% = 2955000000
	// miner3: 20% = 1970000000
	
	if len(rewards) != 3 {
		t.Fatalf("expected 3 rewards, got %d", len(rewards))
	}
	
	miner1 := findReward(rewards, "prl1miner1")
	if miner1.Amount != 4925000000 {
		t.Errorf("miner1: expected 4925000000, got %d", miner1.Amount)
	}
	
	miner2 := findReward(rewards, "prl1miner2")
	if miner2.Amount != 2955000000 {
		t.Errorf("miner2: expected 2955000000, got %d", miner2.Amount)
	}
	
	miner3 := findReward(rewards, "prl1miner3")
	if miner3.Amount != 1970000000 {
		t.Errorf("miner3: expected 1970000000, got %d", miner3.Amount)
	}
}

func findReward(rewards []Reward, address string) *Reward {
	for _, r := range rewards {
		if r.Address == address {
			return &r
		}
	}
	return nil
}
