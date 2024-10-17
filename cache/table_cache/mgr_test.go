package table_cache

import (
	"errors"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
	"time"
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

func GenResourcesModel() any {
	models := make([]*ResourceModel, 0)
	return &models
}

func TestTableCacheMgr_AcquireCacheOp(t *testing.T) {
	db := initDB()

	mgr := NewTableCacheMgr(db)
	defer mgr.Close()

	config := TablePullConfig{
		TableName:      "resource",
		Condition:      map[string]string{"resource_type": "1"},
		UpdateInterval: 0,
		ModelGen:       GenResourcesModel,
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

func TestTableCacheMgr_AcquireCacheOpWithUpdateInterval(t *testing.T) {
	db := initDB()

	mgr := NewTableCacheMgr(db)
	defer mgr.Close()

	config := TablePullConfig{
		TableName:      "resource",
		Condition:      map[string]string{"resource_type": "1"},
		ModelGen:       GenResourcesModel,
		UpdateInterval: time.Second,
	}

	op, err := mgr.AcquireCacheOp(config)
	if err != nil {
		t.Fatalf("failed to acquire cache op: %v", err)
	}

	if op == nil {
		t.Fatalf("cache op is nil")
	}

	rd, oldVersion := op.GetData()
	if rd == nil {
		t.Fatalf("cache data is nil")
	}
	resources, ok := rd.(*[]*ResourceModel)
	if !ok {
		t.Fatalf("cache data is not []*ResourceModel")
	}
	if len(*resources) != 4 {
		t.Fatalf("cache data length is not 4, got %d", len(*resources))
	}

	// insert resource_type = 2 data
	rm1 := &ResourceModel{
		ResourceType: 2,
		ResourceID:   1,
		Name:         "test1",
		Sort:         1,
	}
	err = db.Create(rm1).Error
	if err != nil {
		t.Fatalf("failed to create resource type 2: %v", err)
	}
	defer db.Delete(rm1)

	// wait for update interval
	time.Sleep(2 * time.Second)

	// check cache data
	rd1, newVersion := op.GetData()
	if rd1 == nil {
		t.Fatalf("cache data is nil")
	}
	if oldVersion != newVersion {
		t.Fatalf("cache data version is not equal")
	}
	resources1, ok := rd1.(*[]*ResourceModel)
	if !ok {
		t.Fatalf("cache data is not []*ResourceModel")
	}
	if len(*resources1) != 4 {
		t.Fatalf("cache data length is not 4")
	}

	// insert resource_type = 1 data
	rm2 := &ResourceModel{
		ResourceType: 1,
		ResourceID:   2,
		Name:         "test2",
		Sort:         2,
	}
	err = db.Create(rm2).Error
	if err != nil {
		t.Fatalf("failed to create resource type 1: %v", err)
	}
	defer db.Delete(rm2)

	// wait for update interval
	time.Sleep(2 * time.Second)

	// check cache data
	rd2, newVersion := op.GetData()
	if rd2 == nil {
		t.Fatalf("cache data is nil")
	}
	if oldVersion == newVersion {
		t.Fatalf("cache data version is equal")
	}
	resources2, ok := rd2.(*[]*ResourceModel)
	if !ok {
		t.Fatalf("cache data is not []*ResourceModel")
	}
	if len(*resources2) != 5 {
		t.Fatalf("cache data length is not 5")
	}
}

func TestCheckPullConfigValid(t *testing.T) {
	type testData struct {
		cfg         TablePullConfig
		ExpectedErr error
	}

	tests := []testData{
		{
			cfg: TablePullConfig{
				TableName: "resource",
				ModelGen:  GenResourcesModel,
			},
			ExpectedErr: nil,
		},
		{
			cfg: TablePullConfig{
				TableName: "",
				ModelGen:  GenResourcesModel,
			},
			ExpectedErr: ErrEmptyTableName,
		},
		{
			cfg: TablePullConfig{
				TableName: "resource",
				ModelGen:  nil,
			},
			ExpectedErr: ErrEmptyModelGen,
		},
		{
			cfg: TablePullConfig{
				TableName: "resource",
				ModelGen:  func() any { return nil },
			},
			ExpectedErr: ErrEmptyModelGen,
		},
		{
			cfg: TablePullConfig{
				TableName: "resource",
				ModelGen:  func() any { return 1 },
			},
			ExpectedErr: ErrModelGenUnexpectedVar,
		},
		{
			cfg: TablePullConfig{
				TableName: "resource",
				ModelGen:  func() any { return &[]int{} },
			},
			ExpectedErr: nil,
		},
		{
			cfg: TablePullConfig{
				TableName: "resource",
				ModelGen:  func() any { x := 1; return &x },
			},
			ExpectedErr: ErrModelGenUnexpectedVar,
		},
	}

	for _, test := range tests {
		err := checkPullConfigValid(test.cfg)
		if !errors.Is(err, test.ExpectedErr) {
			t.Fatalf("check pull config valid failed, expected %v, got %v", test.ExpectedErr, err)
		}
	}
}
