package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"gopkg.in/yaml.v3"
)

const defaultConfigPath = "config/database.yaml"

type sourceConfig struct {
	URI             string `yaml:"uri"`
	DefaultDatabase string `yaml:"default_database"`
}

type fileConfig struct {
	Sources map[string]sourceConfig `yaml:"sources"`
}

type mongoSource struct {
	client          *mongo.Client
	defaultDatabase string
}

type sourceRegistry struct {
	sources map[string]mongoSource
}

// loadSources 在服务启动时验证配置及连接。一个 source 固定对应一个连接，
// database 参数只选择该连接有权访问的数据库，不会创建新连接或改变认证身份。
func loadSources(configPath string) (*sourceRegistry, error) {
	if configPath == "" {
		configPath = os.Getenv("MONGODB_CONFIG_PATH")
	}
	if configPath == "" {
		configPath = defaultConfigPath
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取 MongoDB 配置失败: %w", err)
	}
	var cfg fileConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("解析 MongoDB 配置失败: %w", err)
	}
	if len(cfg.Sources) == 0 {
		return nil, errors.New("MongoDB 配置至少需要一个 source")
	}

	registry := &sourceRegistry{sources: make(map[string]mongoSource, len(cfg.Sources))}
	for name, entry := range cfg.Sources {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(entry.URI) == "" {
			registry.close()
			return nil, fmt.Errorf("source %q 缺少名称或 uri", name)
		}
		connectCtx, connectCancel := context.WithTimeout(context.Background(), 5*time.Second)
		client, err := mongo.Connect(connectCtx, options.Client().ApplyURI(entry.URI))
		connectCancel()
		if err != nil {
			registry.close()
			return nil, fmt.Errorf("source %q 连接配置无效: %w", name, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = client.Ping(ctx, readpref.Primary())
		cancel()
		if err != nil {
			_ = client.Disconnect(context.Background())
			registry.close()
			return nil, fmt.Errorf("source %q 连接或认证失败: %w", name, err)
		}
		registry.sources[name] = mongoSource{client: client, defaultDatabase: entry.DefaultDatabase}
	}
	return registry, nil
}

func (r *sourceRegistry) close() {
	for _, source := range r.sources {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = source.client.Disconnect(ctx)
		cancel()
	}
}

func (r *sourceRegistry) names() []string {
	names := make([]string, 0, len(r.sources))
	for name := range r.sources {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// resolve 仅在配置只有一个 source 时允许省略 source，避免多源环境误查。
func (r *sourceRegistry) resolve(sourceName, database string) (string, *mongo.Client, string, error) {
	if sourceName == "" && len(r.sources) == 1 {
		for name := range r.sources {
			sourceName = name
		}
	}
	source, ok := r.sources[sourceName]
	if !ok {
		return "", nil, "", fmt.Errorf("未知的 source %q，可选值: %v", sourceName, r.names())
	}
	if database == "" {
		database = source.defaultDatabase
	}
	return sourceName, source.client, database, nil
}
