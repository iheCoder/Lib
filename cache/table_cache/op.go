package table_cache

type TableCacheOp struct {
	IDxData map[string]any
	data    any
	config  *TablePullConfig
	version int64
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
