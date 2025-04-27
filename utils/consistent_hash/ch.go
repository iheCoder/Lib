package consistent_hash

import (
	"sort"
	"strconv"
)

// HashRing 结构体
type HashRing struct {
	hashMap      map[uint32]string // 哈希值到节点名称映射
	sortedHashes []uint32          // 有序哈希值列表
	virtualNodes int               // 每个节点对应的虚拟节点数
}

// NewHashRing 创建哈希环
func NewHashRing(virtualNodes int) *HashRing {
	return &HashRing{
		hashMap:      make(map[uint32]string),
		virtualNodes: virtualNodes,
	}
}

// AddNode 添加节点及其虚拟节点
func (h *HashRing) AddNode(node string) {

	for i := 0; i < h.virtualNodes; i++ {
		virtualNode := node + "#" + strconv.Itoa(i)
		hash := hashKey(virtualNode)
		h.hashMap[hash] = node
		h.sortedHashes = append(h.sortedHashes, hash)
	}

	sort.Slice(h.sortedHashes, func(i, j int) bool { return h.sortedHashes[i] < h.sortedHashes[j] })
}

// GetNode 根据key查找节点
func (h *HashRing) GetNode(key string) string {
	if len(h.sortedHashes) == 0 {
		return ""
	}

	hash := hashKey(key)
	idx := sort.Search(len(h.sortedHashes), func(i int) bool {
		return h.sortedHashes[i] >= hash
	})
	if idx == len(h.sortedHashes) {
		idx = 0
	}

	return h.hashMap[h.sortedHashes[idx]]
}

// hashKey 使用FNV-1a算法计算哈希
func hashKey(key string) uint32 {
	var (
		hash  uint32 = 2166136261
		prime uint32 = 16777619
	)
	for _, c := range key {
		hash ^= uint32(c)
		hash *= prime
	}
	return hash
}
