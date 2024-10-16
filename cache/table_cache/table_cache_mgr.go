package table_cache

import (
	"gorm.io/gorm"
	"sort"
	"strings"
)

type TablePullConfig struct {
	TableName   string
	Condition   map[string]string
	TableModels []any
}

type TableCacheMgr struct {
	data    map[string]any
	db      *gorm.DB
	configs []TablePullConfig
}

func NewTableCacheMgr(db *gorm.DB) *TableCacheMgr {
	return &TableCacheMgr{
		data: make(map[string]any),
		db:   db,
	}
}

func (mgr *TableCacheMgr) AcquireCacheOp(config TablePullConfig) (*TableCacheOp, error) {
	// check if the data is already in cache
	// if not, pull data
	key := generateItemKey(config.TableName, config.Condition)
	if _, ok := mgr.data[key]; !ok {
		err := mgr.pullTableData(config, key)
		if err != nil {
			return nil, err
		}
	}

	// return cache op
	data := mgr.data[key]
	return &TableCacheOp{
		data:   data,
		config: &config,
	}, nil
}

func (mgr *TableCacheMgr) WithTablePullConfigs(configs ...TablePullConfig) {
	mgr.configs = configs
}

func (mgr *TableCacheMgr) PullData() error {
	for _, config := range mgr.configs {
		// check if the data is already in cache
		key := generateItemKey(config.TableName, config.Condition)
		if _, ok := mgr.data[key]; ok {
			continue
		}

		// pull data
		err := mgr.pullTableData(config, key)
		if err != nil {
			return err
		}
	}

	return nil
}

func (mgr *TableCacheMgr) pullTableData(config TablePullConfig, key string) error {
	// query all data
	err := mgr.db.Where(config.Condition).Find(config.TableModels).Error
	if err != nil {
		return err
	}

	// fill data
	mgr.data[key] = config.TableModels

	// clean config data
	config.TableModels = nil

	return nil
}

func generateItemKey(tableName string, conditions map[string]string) string {
	// acquire all keys and sort them
	keys := make([]string, 0, len(conditions))
	for key := range conditions {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// generate key
	key := tableName
	var sb strings.Builder
	sb.WriteString(key + ":")
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(conditions[k])
		sb.WriteString("&")
	}

	// remove the last "&"
	return sb.String()[:sb.Len()-1]
}
