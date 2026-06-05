package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

type RedisStore struct {
	client *redis.Client
	ctx    context.Context
}

func NewRedisStore(host string, port int, password string, db int) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", host, port),
		Password: password,
		DB:       db,
	})
	
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}
	
	return &RedisStore{
		client: client,
		ctx:    ctx,
	}, nil
}

func (r *RedisStore) Close() error {
	return r.client.Close()
}

func (r *RedisStore) SetCurrentJob(jobID string, job interface{}) error {
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return r.client.Set(r.ctx, "pool:current_job", data, 10*time.Minute).Err()
}

func (r *RedisStore) GetCurrentJob() (string, error) {
	return r.client.Get(r.ctx, "pool:current_job").Result()
}

func (r *RedisStore) IncrPoolHashrate(hashrate float64) error {
	return r.client.IncrByFloat(r.ctx, "pool:hashrate", hashrate).Err()
}

func (r *RedisStore) GetPoolHashrate() (float64, error) {
	return r.client.Get(r.ctx, "pool:hashrate").Float64()
}

func (r *RedisStore) RecordMinerHashrate(address string, hashrate float64) error {
	key := fmt.Sprintf("miner:%s:hashrate", address)
	pipe := r.client.Pipeline()
	pipe.Set(r.ctx, key, hashrate, 5*time.Minute)
	pipe.ZAdd(r.ctx, "pool:miners", &redis.Z{Score: hashrate, Member: address})
	_, err := pipe.Exec(r.ctx)
	return err
}

func (r *RedisStore) GetMinerHashrate(address string) (float64, error) {
	key := fmt.Sprintf("miner:%s:hashrate", address)
	return r.client.Get(r.ctx, key).Float64()
}

func (r *RedisStore) GetTopMiners(limit int64) ([]string, error) {
	return r.client.ZRevRange(r.ctx, "pool:miners", 0, limit-1).Result()
}

func (r *RedisStore) IncrShareCount(address string) error {
	key := fmt.Sprintf("miner:%s:shares", address)
	return r.client.Incr(r.ctx, key).Err()
}

func (r *RedisStore) GetShareCount(address string) (int64, error) {
	key := fmt.Sprintf("miner:%s:shares", address)
	return r.client.Get(r.ctx, key).Int64()
}

func (r *RedisStore) SetWorkerOnline(address, worker string) error {
	key := fmt.Sprintf("miner:%s:workers", address)
	return r.client.SAdd(r.ctx, key, worker).Err()
}

func (r *RedisStore) GetOnlineWorkers(address string) ([]string, error) {
	key := fmt.Sprintf("miner:%s:workers", address)
	return r.client.SMembers(r.ctx, key).Result()
}
