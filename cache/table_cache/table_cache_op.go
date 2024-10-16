package table_cache

type TableCacheOp struct {
	IDxData map[string]any
	data    any
	config  *TablePullConfig
}

func (i *TableCacheOp) GetModelByID(id string) any {
	return i.IDxData[id]
}

func (i *TableCacheOp) GetData() any {
	return i.data
}
