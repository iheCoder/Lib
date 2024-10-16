package table_cache

import (
	"gorm.io/gorm"
)

type TablePullConfig struct {
	TableName   string
	Condition   map[string]string
	TableModels []any
}

type TableCacheMgr struct {
	data       map[string]any
	tableModel []any
	tableName  string
	db         *gorm.DB
	configs    []TablePullConfig
}

func NewTableCacheMgr(db *gorm.DB, tableModel []any, tableName string) *TableCacheMgr {
	return &TableCacheMgr{
		data:       make(map[string]any),
		tableModel: tableModel,
		db:         db,
		tableName:  tableName,
	}
}

func (mgr *TableCacheMgr) WithTablePullConfigs(configs ...TablePullConfig) {
	mgr.configs = configs
}

func (mgr *TableCacheMgr) PullData() error {
	// query all data
	err := mgr.db.Find(mgr.tableModel).Error
	if err != nil {
		return err
	}

	// fill data
	mgr.data = make(map[string]any)
	key := mgr.tableName
	for _, item := range mgr.tableModel {
		mgr.data[key] = item
	}

	return nil
}
