package table_cache

import (
	"crypto/md5"
	"encoding/json"
)

type TableCacheOp struct {
	IDxData map[string]any
	data    any
	config  *TablePullConfig
	version int64
	hash    string
}

func NewTableCacheOp(config *TablePullConfig) *TableCacheOp {
	return &TableCacheOp{
		IDxData: make(map[string]any),
		config:  config,
	}
}

func (i *TableCacheOp) SetData(data any) {
	newHash := genKey(data)
	if newHash == i.hash {
		return
	}

	i.data = data
	i.version++
	i.hash = newHash
}

func (i *TableCacheOp) GetModelByID(id string) any {
	return i.IDxData[id]
}

func (i *TableCacheOp) GetData() (any, int64) {
	return i.data, i.version
}

func (i *TableCacheOp) CheckDataUpdated(version int64) bool {
	return i.version != version
}

func genKey(data any) string {
	ds, _ := json.Marshal(data)
	hash := md5.New()
	hash.Write(ds)
	return string(hash.Sum(nil))
}
