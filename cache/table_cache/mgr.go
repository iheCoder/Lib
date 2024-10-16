package table_cache

import (
	"gorm.io/gorm"
	"sort"
	"strings"
	"time"
)

type TablePullConfig struct {
	TableName      string
	Condition      map[string]string
	TableModels    []any
	UpdateInterval time.Duration
	Selects        []string
}

type TableCacheMgr struct {
	db           *gorm.DB
	ops          map[string]*TableCacheOp
	cancelSignal chan struct{}
}

func NewTableCacheMgr(db *gorm.DB) *TableCacheMgr {
	mgr := &TableCacheMgr{
		db:           db,
		ops:          make(map[string]*TableCacheOp),
		cancelSignal: make(chan struct{}),
	}

	go mgr.startUpdateOpsData()

	return mgr
}

func (mgr *TableCacheMgr) AcquireCacheOp(config TablePullConfig) (*TableCacheOp, error) {
	// check if the data is already in cache
	// if not, pull data
	key := generateItemKey(config.TableName, config.Condition)
	if _, ok := mgr.ops[key]; !ok {
		err := mgr.pullTableData(config, key)
		if err != nil {
			return nil, err
		}
	}

	// return cache op
	op := mgr.ops[key]
	return op, nil
}

func (mgr *TableCacheMgr) pullTableData(config TablePullConfig, key string) error {
	// query all data
	if len(config.Selects) == 0 {
		config.Selects = []string{"*"}
	}
	err := mgr.db.Select(config.Selects).Where(config.Condition).Find(config.TableModels).Error
	if err != nil {
		return err
	}

	// fill op
	op := NewTableCacheOp(&config)
	op.SetData(config.TableModels)
	mgr.ops[key] = op

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

func (mgr *TableCacheMgr) Close() {
	close(mgr.cancelSignal)
}
