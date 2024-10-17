package table_cache

import "errors"

var (
	ErrEmptyTableName        = errors.New("table name is empty")
	ErrEmptyModelGen         = errors.New("model generator is empty")
	ErrModelGenUnexpectedVar = errors.New("the var that model generator is not a pointer of struct slice")
)
