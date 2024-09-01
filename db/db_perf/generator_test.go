package db_perf

import (
	"encoding/json"
	"gorm.io/gorm"
	"math/rand"
	"reflect"
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
	Price      float64 `gorm:"type:decimal(8,2)"`
	Info       string  `gorm:"type:json" gen:"key:extra"`
}

type PerfTestExtra struct {
	Score  int     `json:"score"`
	Degree string  `json:"degree"`
	Weight float64 `json:"weight"`
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
	err = g.InsertMassData(&PerfTest{}, count)
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

func TestMassDataGenerator_GenerateDataSetRange(t *testing.T) {
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
	strOptions := []string{"你好", "hello", "こんにちは", "안녕", "Bonjour"}
	g := NewMassDataGenerator(tx, WithRanges(map[reflect.Kind]valueRange{
		reflect.String: {strOptions: strOptions},
	}))
	err = g.InsertMassData(&PerfTest{}, count)
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

	// get last data check if name is in strOptions
	var lastData PerfTest
	err = tx.Last(&lastData).Error
	if err != nil {
		t.Error(err)
		return
	}

	strOptMap := make(map[string]bool)
	for _, opt := range strOptions {
		strOptMap[opt] = true
	}

	if !strOptMap[lastData.Name] {
		t.Errorf("expect %v, got %v", strOptions, lastData.Name)
		return
	}

	t.Log("success")
}

func TestMassDataGenerator_GenerateDataSetValueGenerator(t *testing.T) {
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
	valGens := map[string]valueGenerator{
		"extra": func() any {
			score := rand.Intn(700) + 1
			degree := []string{"小学", "初中", "高中", "本科", "硕士", "博士"}[rand.Intn(6)]
			weight := float64(rand.Intn(150)+50) + rand.Float64()
			extra := PerfTestExtra{
				Score:  score,
				Degree: degree,
				Weight: weight,
			}

			eb, _ := json.Marshal(extra)
			return string(eb)
		},
	}
	g := NewMassDataGenerator(tx, WithValueGenerators(valGens))
	err = g.InsertMassData(&PerfTest{}, count)
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

	// get last data check if name is in strOptions
	var lastData PerfTest
	err = tx.Last(&lastData).Error
	if err != nil {
		t.Error(err)
		return
	}

	var extra PerfTestExtra
	err = json.Unmarshal([]byte(lastData.Info), &extra)
	if err != nil {
		t.Error(err)
		return
	}

	if extra.Score < 1 || extra.Score > 700 {
		t.Errorf("expect 1-700, got %d", extra.Score)
		return
	}
	if extra.Degree == "" {
		t.Errorf("expect not empty, got %s", extra.Degree)
		return
	}
	if extra.Weight < 50 || extra.Weight > 200 {
		t.Errorf("expect 50-200, got %f", extra.Weight)
		return
	}

	t.Log("success")
}
