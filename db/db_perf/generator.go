package db_perf

import (
	"fmt"
	"gorm.io/gorm"
	"math"
	"math/rand"
	"reflect"
	"strconv"
)

const (
	maxStringLength = 20
)

var (
	// I think even table type is bigint, although the max value is 2^32-1 is enough
	defaultValueRanges = map[reflect.Kind]valueRange{
		reflect.Int:     {lower: 0, upper: math.MaxInt32},
		reflect.Int8:    {lower: 0, upper: math.MaxInt8},
		reflect.Int16:   {lower: 0, upper: math.MaxInt16},
		reflect.Int32:   {lower: 0, upper: math.MaxInt32},
		reflect.Int64:   {lower: 0, upper: math.MaxInt32},
		reflect.Uint:    {lower: 0, upper: math.MaxUint32},
		reflect.Uint8:   {lower: 0, upper: math.MaxUint8},
		reflect.Uint16:  {lower: 0, upper: math.MaxUint16},
		reflect.Uint32:  {lower: 0, upper: math.MaxUint32},
		reflect.Uint64:  {lower: 0, upper: math.MaxUint32},
		reflect.Float32: {lowerFloat: 0.0, upperFloat: math.MaxFloat32},
		reflect.Float64: {lowerFloat: 0.0, upperFloat: math.MaxFloat32},
		reflect.String:  {strLen: maxStringLength},
	}
)

type valueGenerator func(model any)
type valueRange struct {
	// int options
	lower, upper int

	// float options
	lowerFloat, upperFloat float64

	// random string options
	strOptions []string
	strLen     int
}

type MassDataGenerator struct {
	tx     *gorm.DB
	ranges map[reflect.Kind]valueRange
}

type OptionFunc func(*MassDataGenerator)

func WithRanges(ranges map[reflect.Kind]valueRange) OptionFunc {
	return func(g *MassDataGenerator) {
		g.ranges = ranges
	}
}

func NewMassDataGenerator(db *gorm.DB, opts ...OptionFunc) *MassDataGenerator {
	g := &MassDataGenerator{
		tx:     db,
		ranges: defaultValueRanges,
	}

	for _, opt := range opts {
		opt(g)
	}

	return g
}

func (g *MassDataGenerator) InsertMassData(model any, count int) error {
	for i := 0; i < count; i++ {
		g.generateValuesForModel(model)
		if err := g.tx.Create(model).Error; err != nil {
			return err
		}
	}
	return nil
}

func (g *MassDataGenerator) UpdateRange(kind reflect.Kind, r valueRange) {
	g.ranges[kind] = r
}

// generateValuesForModel generates values for model in reflection way
func (g *MassDataGenerator) generateValuesForModel(model any) {
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
				g.setInt(field.Kind(), field) // 设置一个随机整数
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				g.setInt(field.Kind(), field) // 设置一个随机整数
			case reflect.Float32, reflect.Float64:
				g.setFloat(field.Kind(), field) // 设置一个随机浮点数
			case reflect.String:
				g.setString(field.Kind(), field, fieldType) // 设置一个随机字符串
			case reflect.Bool:
				field.SetBool(rand.Intn(2) == 1) // 随机布尔值
			}
		} else {
			fmt.Printf("字段 %s 无法设置\n", fieldType.Name)
		}
	}
}

// setInt get value by int kind ranges
func (g *MassDataGenerator) setInt(kind reflect.Kind, field reflect.Value) {
	val := int64(rand.Intn(g.ranges[kind].upper-g.ranges[kind].lower) + g.ranges[kind].lower)
	field.SetInt(val)
}

// setFloat get value by float kind ranges
func (g *MassDataGenerator) setFloat(kind reflect.Kind, field reflect.Value) {
	val := rand.Float64()*(g.ranges[kind].upperFloat-g.ranges[kind].lowerFloat) + g.ranges[kind].lowerFloat
	field.SetFloat(val)
}

// setString get value by string kind ranges
func (g *MassDataGenerator) setString(kind reflect.Kind, field reflect.Value, fieldType reflect.StructField) {
	// if has options, select one from options
	if len(g.ranges[kind].strOptions) > 0 {
		field.SetString(randSelectOneFromOptions(g.ranges[kind].strOptions))
		return
	}

	// get gorm tag size
	size, _ := strconv.Atoi(fieldType.Tag.Get("size"))

	// get min str len if size > 0
	strLen := g.ranges[kind].strLen
	if size > 0 && size < strLen {
		strLen = size
	}

	// set random string
	field.SetString(generateRandomString(rand.Intn(strLen) + 1))
}

func randSelectOneFromOptions(options []string) string {
	return options[rand.Intn(len(options))]
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
