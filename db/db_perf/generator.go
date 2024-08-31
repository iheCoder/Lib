package db_perf

import (
	"fmt"
	"gorm.io/gorm"
	"math/rand"
	"reflect"
)

const (
	maxRandIntUpperLimit = 1000_000
	maxStringLength      = 50
)

type MassDataGenerator struct {
	tx *gorm.DB
}

func NewMassDataGenerator(db *gorm.DB) *MassDataGenerator {
	return &MassDataGenerator{tx: db}
}

func (g *MassDataGenerator) GenerateDataWithoutDataSet(model any, count int) error {
	for i := 0; i < count; i++ {
		generateValuesForModel(model)
		if err := g.tx.Create(model).Error; err != nil {
			return err
		}
	}
	return nil
}

// generateValuesForModel generates values for model in reflection way
func generateValuesForModel(model any) {
	v := reflect.ValueOf(model)

	// 确保传入的是一个指针并且是指向结构体的
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		fmt.Println("传入的必须是结构体指针")
		return
	}

	v = v.Elem()  // 获取结构体的Value
	t := v.Type() // 获取结构体的Type

	// 遍历结构体的字段
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		// 检查是否为主键，主键跳过，并设置零值
		if fieldType.Tag.Get("gorm") == "primaryKey" {
			field.Set(reflect.Zero(field.Type()))
			continue
		}

		// 根据字段类型设置随机值
		if field.CanSet() { // 检查字段是否可以被设置
			switch field.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				field.SetInt(int64(rand.Intn(maxRandIntUpperLimit))) // 设置一个随机整数
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				field.SetUint(uint64(rand.Intn(maxRandIntUpperLimit))) // 设置一个随机无符号整数
			case reflect.Float32, reflect.Float64:
				field.SetFloat(rand.Float64() * maxRandIntUpperLimit) // 设置一个随机浮点数
			case reflect.String:
				field.SetString(generateRandomString(rand.Intn(maxStringLength) + 1)) // 设置一个随机字符串
			case reflect.Bool:
				field.SetBool(rand.Intn(2) == 1) // 随机布尔值
			}
		} else {
			fmt.Printf("字段 %s 无法设置\n", fieldType.Name)
		}
	}
}

// generateRandomString 生成一个随机字符串
func generateRandomString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	result := make([]rune, n)
	for i := range result {
		result[i] = letters[rand.Intn(len(letters))]
	}
	return string(result)
}
