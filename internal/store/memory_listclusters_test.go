package store

import (
    "context"
    "testing"
    "github.com/vaheed/kubenova/pkg/types"
)

func TestMemory_ListClusters_LabelSelectorAndCursor(t *testing.T) {
    m := NewMemory()
    // seed
    _, _ = m.CreateCluster(context.Background(), types.Cluster{Name: "c1", Labels: map[string]string{"env":"dev"}}, "enc")
    _, _ = m.CreateCluster(context.Background(), types.Cluster{Name: "c2", Labels: map[string]string{"env":"prod"}}, "enc")
    _, _ = m.CreateCluster(context.Background(), types.Cluster{Name: "c3", Labels: map[string]string{"env":"prod","tier":"gold"}}, "enc")

    items, next, err := m.ListClusters(context.Background(), 2, "", "")
    if err != nil || len(items) != 2 || next == "" { t.Fatalf("page1 err=%v len=%d next=%s", err, len(items), next) }
    items2, _, err := m.ListClusters(context.Background(), 2, next, "")
    if err != nil || len(items2) < 1 { t.Fatalf("page2 err=%v len=%d", err, len(items2)) }

    prods, _, err := m.ListClusters(context.Background(), 10, "", "env=prod")
    if err != nil || len(prods) != 2 { t.Fatalf("env=prod expected 2 got %d", len(prods)) }
    gold, _, err := m.ListClusters(context.Background(), 10, "", "env=prod,tier=gold")
    if err != nil || len(gold) != 1 { t.Fatalf("tier=gold expected 1 got %d", len(gold)) }
}

