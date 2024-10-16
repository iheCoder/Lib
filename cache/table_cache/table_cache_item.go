package table_cache

type TableCacheItem struct {
	TableName  string
	Conditions map[string]string
	IDxData    map[string]any
}

func (i *TableCacheItem) GetModelByID(id string) any {
	return i.IDxData[id]
}
