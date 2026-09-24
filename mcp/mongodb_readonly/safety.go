package main

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
)

const (
	defaultLimit   = 100
	maxLimit       = 500
	maxSkip        = 100000
	maxInputBytes  = 64 * 1024
	maxOutputBytes = 1024 * 1024
)

// 只接受明确的读取型聚合阶段。新增阶段时需确认其副作用和嵌套管道结构。
var readStages = map[string]bool{
	"$match": true, "$project": true, "$sort": true, "$skip": true,
	"$limit": true, "$group": true, "$count": true, "$unwind": true,
	"$addFields": true, "$set": true, "$unset": true,
	"$replaceRoot": true, "$replaceWith": true,
	"$sortByCount": true, "$bucket": true, "$bucketAuto": true,
	"$sample": true, "$lookup": true, "$facet": true,
	"$unionWith": true, "$graphLookup": true,
}

var forbiddenOperators = map[string]bool{
	"$where": true, "$function": true, "$accumulator": true,
}

func parseDocument(input, parameter string) (bson.D, error) {
	if strings.TrimSpace(input) == "" {
		return bson.D{}, nil
	}
	if len(input) > maxInputBytes {
		return nil, fmt.Errorf("%s 超过 %d 字节", parameter, maxInputBytes)
	}
	var doc bson.D
	if err := bson.UnmarshalExtJSON([]byte(input), false, &doc); err != nil {
		return nil, fmt.Errorf("%s 不是有效的 Extended JSON 文档: %w", parameter, err)
	}
	if err := rejectExecutableOperators(doc); err != nil {
		return nil, fmt.Errorf("%s: %w", parameter, err)
	}
	return doc, nil
}

func parsePipeline(input string) (bson.A, error) {
	if strings.TrimSpace(input) == "" {
		return nil, errors.New("pipeline 不能为空")
	}
	if len(input) > maxInputBytes {
		return nil, fmt.Errorf("pipeline 超过 %d 字节", maxInputBytes)
	}
	var pipeline bson.A
	if err := bson.UnmarshalExtJSON([]byte(input), false, &pipeline); err != nil {
		return nil, fmt.Errorf("pipeline 不是有效的 Extended JSON 数组: %w", err)
	}
	if err := validatePipeline(pipeline, 0); err != nil {
		return nil, err
	}
	return pipeline, nil
}

func validatePipeline(pipeline bson.A, depth int) error {
	if len(pipeline) == 0 || len(pipeline) > 30 || depth > 4 {
		return errors.New("pipeline 阶段数或嵌套层数超出限制")
	}
	for _, item := range pipeline {
		stage, ok := item.(bson.D)
		if !ok || len(stage) != 1 {
			return errors.New("pipeline 的每个阶段必须是仅含一个操作符的文档")
		}
		operator := stage[0].Key
		if !readStages[operator] {
			return fmt.Errorf("不允许聚合阶段 %q", operator)
		}
		if err := rejectExecutableOperators(stage[0].Value); err != nil {
			return err
		}

		// 这些阶段可以包含二级管道；同样执行只读阶段白名单。
		switch operator {
		case "$lookup", "$unionWith":
			if doc, ok := stage[0].Value.(bson.D); ok {
				if nested, found := documentField(doc, "pipeline"); found {
					stages, ok := nested.(bson.A)
					if !ok {
						return errors.New("嵌套 pipeline 必须是数组")
					}
					if err := validatePipeline(stages, depth+1); err != nil {
						return err
					}
				}
			}
		case "$facet":
			facets, ok := stage[0].Value.(bson.D)
			if !ok {
				return errors.New("$facet 必须是文档")
			}
			for _, facet := range facets {
				stages, ok := facet.Value.(bson.A)
				if !ok {
					return errors.New("$facet 的每个分支必须是 pipeline 数组")
				}
				if err := validatePipeline(stages, depth+1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func documentField(doc bson.D, name string) (any, bool) {
	for _, field := range doc {
		if field.Key == name {
			return field.Value, true
		}
	}
	return nil, false
}

// 禁止服务端 JavaScript 表达式，避免读查询变成任意脚本执行入口。
func rejectExecutableOperators(value any) error {
	switch typed := value.(type) {
	case bson.D:
		seen := make(map[string]bool, len(typed))
		for _, field := range typed {
			if seen[field.Key] {
				return fmt.Errorf("文档包含重复字段 %q", field.Key)
			}
			seen[field.Key] = true
			if forbiddenOperators[field.Key] {
				return fmt.Errorf("不允许操作符 %q", field.Key)
			}
			if err := rejectExecutableOperators(field.Value); err != nil {
				return err
			}
		}
	case bson.A:
		for _, item := range typed {
			if err := rejectExecutableOperators(item); err != nil {
				return err
			}
		}
	}
	return nil
}

func boundedInteger(value any, fallback, maximum int, name string) (int, error) {
	if value == nil {
		return fallback, nil
	}
	number, ok := value.(float64)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) || number != math.Trunc(number) || number < 0 || number > float64(maximum) {
		return 0, fmt.Errorf("%s 必须是 0 到 %d 的整数", name, maximum)
	}
	return int(number), nil
}
