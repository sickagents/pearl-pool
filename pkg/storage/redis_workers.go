package storage

// GetOnlineWorkers returns list of online workers for address
func (s *RedisStore) GetOnlineWorkers(address string) ([]string, error) {
	key := "worker:online:" + address
	members, err := s.client.SMembers(s.ctx, key).Result()
	if err != nil {
		return nil, err
	}
	return members, nil
}

// SetWorkerOnline marks worker as online
func (s *RedisStore) SetWorkerOnline(address, worker string) error {
	key := "worker:online:" + address
	return s.client.SAdd(s.ctx, key, worker).Err()
}

// GetShareCount returns total shares submitted by address
func (s *RedisStore) GetShareCount(address string) (int64, error) {
	key := "shares:" + address
	count, err := s.client.Get(s.ctx, key).Int64()
	if err != nil {
		return 0, nil // Return 0 if key doesn't exist
	}
	return count, nil
}

// IncrShareCount increments share count for address
func (s *RedisStore) IncrShareCount(address string) error {
	key := "shares:" + address
	return s.client.Incr(s.ctx, key).Err()
}

// GetTopMiners returns top N miners by hashrate
func (s *RedisStore) GetTopMiners(limit int) ([]string, error) {
	// Get all miner keys
	keys, err := s.client.Keys(s.ctx, "miner:hashrate:*").Result()
	if err != nil {
		return nil, err
	}
	
	// Extract addresses
	var addresses []string
	for _, key := range keys {
		// Remove "miner:hashrate:" prefix
		address := key[15:]
		addresses = append(addresses, address)
	}
	
	if len(addresses) > limit {
		return addresses[:limit], nil
	}
	
	return addresses, nil
}
