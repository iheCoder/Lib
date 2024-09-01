package db_perf

import (
	"fmt"
	"gorm.io/gorm"
	"math"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
)

const (
	maxStringLength = 20
	minStringLength = 1
	genTagName      = "gen"
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
		reflect.String:  {strLenMax: maxStringLength},
	}

	errNotStructPtr = fmt.Errorf("传入的必须是结构体指针")
)

type gormTag struct {
	PrimaryKey bool
	Size       int
	Type       string
}

func parseGormTag(tag string) *gormTag {
	if len(tag) == 0 {
		return nil
	}

	t := gormTag{}
	// search for primary key
	if strings.Contains(tag, "primaryKey") {
		t.PrimaryKey = true
	}

	tags := strings.Split(tag, ";")
	for _, tag := range tags {
		parts := strings.Split(tag, ":")
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "size":
			t.Size, _ = strconv.Atoi(parts[1])
		case "type":
			t.Type = parts[1]
		}
	}
	return &t
}

type valueGenerator func() any
type valueRange struct {
	// int options
	lower, upper int

	// float options
	lowerFloat, upperFloat float64

	// random string options
	strOptions           []string
	strLenMax, strLenMin int
}

type MassDataGenerator struct {
	tx      *gorm.DB
	ranges  map[reflect.Kind]valueRange
	valGens map[string]valueGenerator
}

type OptionFunc func(*MassDataGenerator)

func WithRanges(ranges map[reflect.Kind]valueRange) OptionFunc {
	return func(g *MassDataGenerator) {
		g.ranges = ranges
	}
}

func WithValueGenerators(valGens map[string]valueGenerator) OptionFunc {
	return func(g *MassDataGenerator) {
		g.valGens = valGens
	}
}

func NewMassDataGenerator(db *gorm.DB, opts ...OptionFunc) *MassDataGenerator {
	g := &MassDataGenerator{
		tx:      db,
		ranges:  defaultValueRanges,
		valGens: make(map[string]valueGenerator),
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
func (g *MassDataGenerator) generateValuesForModel(model any) error {
	v := reflect.ValueOf(model)

	// 确保传入的是一个指针并且是指向结构体的
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return errNotStructPtr
	}

	v = v.Elem()  // 获取结构体的Value
	t := v.Type() // 获取结构体的Type

	// 遍历结构体的字段
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		// 检查是否为主键，主键跳过，并设置零值
		gt := parseGormTag(fieldType.Tag.Get("gorm"))
		if gt != nil && gt.PrimaryKey {
			field.Set(reflect.Zero(field.Type()))
			continue
		}

		// 检查是否有gen tag，取出key，并使用对应的生成器生成值
		genTag := fieldType.Tag.Get(genTagName)
		if genTag != "" {
			key := searchKey(genTag)
			if len(key) > 0 {
				if err := g.setValueFromGenerator(field, key); err != nil {
					return err
				}
				continue
			}
		}

		// 根据字段类型设置随机值
		if field.CanSet() { // 检查字段是否可以被设置
			switch field.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				g.setInt(field.Kind(), field) // 设置一个随机整数
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				g.setInt(field.Kind(), field) // 设置一个随机整数
			case reflect.Float32, reflect.Float64:
				g.setFloat(field.Kind(), field, gt) // 设置一个随机浮点数
			case reflect.String:
				g.setString(field.Kind(), field, gt) // 设置一个随机字符串
			case reflect.Bool:
				field.SetBool(rand.Intn(2) == 1) // 随机布尔值
			}
		} else {
			return fmt.Errorf("field %s can't be set", fieldType.Name)
		}
	}
	return nil
}

func searchKey(tag string) string {
	// split tag by ','
	tags := strings.Split(tag, ",")
	for _, t := range tags {
		// split tag by ':'
		parts := strings.Split(t, ":")
		if len(parts) == 2 && parts[0] == "key" {
			return parts[1]
		}
	}

	return ""
}

func (g *MassDataGenerator) setValueFromGenerator(field reflect.Value, key string) error {
	if gen, ok := g.valGens[key]; ok {
		field.Set(reflect.ValueOf(gen()))
		return nil
	}
	return fmt.Errorf("no generator found for key %s", key)
}

// setInt get value by int kind ranges
func (g *MassDataGenerator) setInt(kind reflect.Kind, field reflect.Value) {
	val := int64(rand.Intn(g.ranges[kind].upper-g.ranges[kind].lower) + g.ranges[kind].lower)
	field.SetInt(val)
}

// setFloat get value by float kind ranges
func (g *MassDataGenerator) setFloat(kind reflect.Kind, field reflect.Value, tag *gormTag) {
	upperFloat := g.ranges[kind].upperFloat
	lowerFloat := g.ranges[kind].lowerFloat
	// handle decimal special
	if tag != nil && strings.Contains(tag.Type, "decimal") {
		// remove "decimal(" and ")", get prec and scale
		tp := tag.Type
		tp = tp[8 : len(tp)-1]
		parts := strings.Split(tp, ",")
		scale, _ := strconv.Atoi(parts[0])
		prec, _ := strconv.Atoi(parts[1])
		if scale > 0 {
			upperFloat = math.Pow10(scale)
			val := rand.Float64()*(upperFloat-lowerFloat) + lowerFloat
			val = float64(int(val)) / math.Pow10(prec)
			field.SetFloat(val)
			return
		}
	}

	val := rand.Float64()*(upperFloat-lowerFloat) + lowerFloat
	field.SetFloat(val)
}

// setString get value by string kind ranges
func (g *MassDataGenerator) setString(kind reflect.Kind, field reflect.Value, tag *gormTag) {
	// if has options, select one from options
	if len(g.ranges[kind].strOptions) > 0 {
		field.SetString(randSelectOneFromOptions(g.ranges[kind].strOptions))
		return
	}

	// get min str len if size > 0
	strLenMin := g.ranges[kind].strLenMin
	strLenMax := g.ranges[kind].strLenMax
	if tag != nil && tag.Size > 0 && tag.Size < strLenMax {
		strLenMax = tag.Size
	}

	// set random string
	field.SetString(generateRandomString(rand.Intn(strLenMax-strLenMin) + strLenMin))
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
