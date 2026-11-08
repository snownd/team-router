package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"go.uber.org/zap"

	"github.com/snownd/team-router/internal/crypto"
	"github.com/snownd/team-router/internal/database/dbtest"
	"github.com/snownd/team-router/internal/model"
	"github.com/snownd/team-router/internal/repository"
)

// TestWeightedShuffleConcurrent：并发加权抽样不产生数据竞争（需 go test -race）。
func TestWeightedShuffleConcurrent(t *testing.T) {
	chs := []*Channel{{Weight: 1}, {Weight: 3}, {Weight: 5}, {Weight: 2}, {Weight: 7}}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				out := weightedShuffle(chs)
				if len(out) != len(chs) {
					t.Error("打乱后长度应不变")
					return
				}
			}
		}()
	}
	wg.Wait()
}

// TestWeightedShuffleKeepsMembers：打乱不丢失、不重复成员，且不修改入参。
func TestWeightedShuffleKeepsMembers(t *testing.T) {
	chs := []*Channel{{ID: 1, Weight: 1}, {ID: 2, Weight: 3}, {ID: 3, Weight: 5}}
	out := weightedShuffle(chs)
	seen := map[int64]bool{}
	for _, c := range out {
		seen[c.ID] = true
	}
	if len(seen) != 3 {
		t.Errorf("应保留全部成员: %v", seen)
	}
	if len(chs) != 3 || chs[0].ID != 1 {
		t.Errorf("不应修改入参顺序: %v", chs)
	}
}

// TestSelectForModelConcurrent：并发渠道路由（配合 -race 验证缓存刷新与抽样无竞争）。
// 需 TEST_PG_DSN。
func TestSelectForModelConcurrent(t *testing.T) {
	db := dbtest.Prepare(t)
	ctx := context.Background()
	enc, err := crypto.New("")
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry([]Adapter{NewOpenAIAdapter(), NewAnthropicAdapter()})
	r := NewRouter(repository.NewProviderRepo(db), enc, reg, zap.NewNop())

	prio := []int{10, 10, 10, 5, 5, 1}
	for i := 0; i < len(prio); i++ {
		encKey, _ := enc.Encrypt("sk-p")
		p := model.Provider{
			Name: fmt.Sprintf("p-%d", i), Type: "openai",
			BaseURL: "http://127.0.0.1:1", APIKeyEnc: encKey,
			Models: json.RawMessage(`["m-common"]`),
			Weight: i + 1, Priority: prio[i], Status: 1,
		}
		if err := repository.NewProviderRepo(db).Create(ctx, &p); err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				chans, err := r.SelectForModel(ctx, "m-common")
				if err != nil {
					t.Error(err)
					return
				}
				if len(chans) == 0 {
					t.Error("应至少命中一个渠道")
					return
				}
				// 同优先级内权重高的渠道应整体靠前（抽序，仅验证不 panic 与非空）
			}
		}()
	}
	wg.Wait()
}
