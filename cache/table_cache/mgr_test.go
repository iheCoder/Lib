package table_cache

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
)

const defaultDBPath = "testdata/test.db"

func initDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open(defaultDBPath), &gorm.Config{})
	if err != nil {
		panic(err)
	}

	return db
}

type ResourceModel struct {
	ID           int    `gorm:"primaryKey" column:"id"`
	ResourceType int32  `column:"resource_type"`
	ResourceID   int64  `column:"resource_id"`
	Name         string `column:"name"`
	Sort         int32  `column:"sort"`
}

func (ResourceModel) TableName() string {
	return "resource"
}

func TestTableCacheMgr_AcquireCacheOp(t *testing.T) {
	db := initDB()

	mgr := NewTableCacheMgr(db)
	defer mgr.Close()

	models := make([]*ResourceModel, 0)
	config := TablePullConfig{
		TableName:      "resource",
		Condition:      map[string]string{"resource_type": "1"},
		TableModels:    &models,
		UpdateInterval: 0,
	}

	op, err := mgr.AcquireCacheOp(config)
	if err != nil {
		t.Fatalf("failed to acquire cache op: %v", err)
	}

	if op == nil {
		t.Fatalf("cache op is nil")
	}

	rd, _ := op.GetData()
	if rd == nil {
		t.Fatalf("cache data is nil")
	}
	resources, ok := rd.(*[]*ResourceModel)
	if !ok {
		t.Fatalf("cache data is not []*ResourceModel")
	}
	if len(*resources) != 4 {
		t.Fatalf("cache data length is not 4")
	}
}
