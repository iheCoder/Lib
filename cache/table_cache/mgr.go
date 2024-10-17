package table_cache

import (
	"gorm.io/gorm"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/RussellLuo/timingwheel"
)

type ModelsGenerator func() any

type TablePullConfig struct {
	// TableName is the table name
	TableName string
	// Condition is the where condition
	Condition map[string]string
	// UpdateInterval is the interval to update the data
	// if not set, the data will not be updated
	// the real update interval will add a random time
	UpdateInterval time.Duration
	// Selects is the columns to be selected
	// if not set, all columns will be selected
	// pay attention that if same table and conditions, the last op will overwrite the previous one
	Selects []string
	// ModelGen is the models generator
	ModelGen ModelsGenerator
}

type TableCacheMgr struct {
	db           *gorm.DB
	ops          map[string]*TableCacheOp
	cancelSignal chan struct{}
	tw           *timingwheel.TimingWheel
}

func NewTableCacheMgr(db *gorm.DB) *TableCacheMgr {
	mgr := &TableCacheMgr{
		db:           db,
		ops:          make(map[string]*TableCacheOp),
		cancelSignal: make(chan struct{}),
		tw:           timingwheel.NewTimingWheel(defaultWheelInterval, 60),
	}

	go mgr.startUpdateOpsData()

	return mgr
}

func (mgr *TableCacheMgr) AcquireCacheOp(config TablePullConfig) (*TableCacheOp, error) {
	// check if the op exists
	// if not exists, create a new one
	key := generateItemKey(config.TableName, config.Condition)
	op, ok := mgr.ops[key]
	if !ok {
		// create new op
		op = NewTableCacheOp(&config)
		mgr.ops[key] = op

		// add op to timing wheel
		mgr.addToUpdateWheel(op)
	}

	// update data
	if err := mgr.updateOpData(op); err != nil {
		return nil, err
	}

	return op, nil
}

func checkPullConfigValid(cfg TablePullConfig) error {
	if len(cfg.TableName) == 0 {
		return ErrEmptyTableName
	}
	if cfg.ModelGen == nil {
		return ErrEmptyModelGen
	}

	models := cfg.ModelGen()
	if models == nil {
		return ErrEmptyModelGen
	}
	// check if models is a pointer of struct slice
	if reflect.TypeOf(models).Kind() != reflect.Ptr || reflect.TypeOf(models).Elem().Kind() != reflect.Slice {
		return ErrModelGenUnexpectedVar
	}

	return nil
}

func (mgr *TableCacheMgr) updateOpData(op *TableCacheOp) error {
	// pull data
	data, err := mgr.pullTableData(*op.config)
	if err != nil {
		return err
	}

	// set data
	op.SetData(data)

	return nil
}

func (mgr *TableCacheMgr) pullTableData(config TablePullConfig) (any, error) {
	// check if selects is empty, set to "*"
	if len(config.Selects) == 0 {
		config.Selects = []string{"*"}
	}

	// create models
	models := config.ModelGen()

	// query data
	err := mgr.db.Select(config.Selects).Where(config.Condition).Find(models).Error
	if err != nil {
		return nil, err
	}

	return models, nil
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
