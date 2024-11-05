package table_cache

import (
	"github.com/bytedance/sonic"
	"github.com/cespare/xxhash/v2"
	"time"
)

type DataReviseFunc func(data any) (any, error)

type TableCacheOp struct {
	IDxData  map[string]any
	data     any
	config   *TablePullConfig
	version  int64
	hash     uint64
	size     int
	pullTime time.Time
}

func NewTableCacheOp(config *TablePullConfig) *TableCacheOp {
	return &TableCacheOp{
		IDxData: make(map[string]any),
		config:  config,
	}
}

func (i *TableCacheOp) SetData(rawData any) error {
	newHash, size := i.genKey(rawData)
	if newHash == i.hash {
		return nil
	}

	i.pullTime = time.Now()
	i.data = rawData
	i.version++
	i.hash = newHash
	i.size = size

	return i.reviseData()
}

func (i *TableCacheOp) GetModelByID(id string) any {
	return i.IDxData[id]
}

func (i *TableCacheOp) GetData() (any, int64) {
	return i.data, i.version
}

func (i *TableCacheOp) GetPullTime() time.Time {
	return i.pullTime
}

// GetDataSize returns the size of the data in bytes.
func (i *TableCacheOp) GetDataSize() int {
	return i.size
}

func (i *TableCacheOp) CheckDataUpdated(version int64) bool {
	return i.version != version
}

func (i *TableCacheOp) genKey(data any) (uint64, int) {
	ds, _ := sonic.Marshal(data)
	return xxhash.Sum64(ds), len(ds)
}

func (i *TableCacheOp) reviseData() error {
	if i.config.ReviseFunc != nil {
		rd, err := i.config.ReviseFunc(i.data)
		if err != nil {
			return err
		}
		i.data = rd
	}

	return nil
}
