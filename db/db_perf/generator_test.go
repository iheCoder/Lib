package db_perf

import (
	"gorm.io/gorm"
	"testing"
)
import "gorm.io/driver/mysql"

func initDB() *gorm.DB {
	source := "root:123456@tcp(localhost:3306)/learn"
	db, err := gorm.Open(mysql.Open(source), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	return db
}

type PerfTest struct {
	ID         int     `gorm:"primaryKey"`
	Name       string  `gorm:"size:255"`
	FloatPrice float64 `gorm:"type:float"`
}

func (p *PerfTest) TableName() string {
	return "perf_test"
}

func TestMassDataGenerator_GenerateDataWithoutDataSet(t *testing.T) {
	db := initDB()
	tx := db.Begin()
	defer tx.Rollback()

	count := 10
	// get data len before insert
	var startCount int64
	err := tx.Model(&PerfTest{}).Count(&startCount).Error
	if err != nil {
		t.Error(err)
		return
	}

	// insert data
	g := NewMassDataGenerator(tx)
	err = g.GenerateDataWithoutDataSet(&PerfTest{}, count)
	if err != nil {
		t.Error(err)
		return
	}

	// get data len after insert
	var endCount int64
	err = tx.Model(&PerfTest{}).Count(&endCount).Error
	if err != nil {
		t.Error(err)
	}

	if endCount != startCount+int64(count) {
		t.Errorf("expect %d, got %d", startCount+int64(count), endCount)
		return
	}
}
