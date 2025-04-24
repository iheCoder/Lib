package main

import "testing"

func TestAddGetDelete(t *testing.T) {
	db, err := NewDatabase()
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.db.Close()

	key := "testKey"
	value := "testValue"

	// Add
	err = db.Add(key, value)
	if err != nil {
		t.Errorf("Add failed: %v", err)
	}

	// Get
	got, err := db.Get(key)
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}
	if got != value {
		t.Errorf("Expected value %s, got %s", value, got)
	}

	// Update
	newValue := "newValue"
	err = db.Update(key, newValue)
	if err != nil {
		t.Errorf("Update failed: %v", err)
	}
	got, err = db.Get(key)
	if err != nil {
		t.Errorf("Get after update failed: %v", err)
	}
	if got != newValue {
		t.Errorf("Expected updated value %s, got %s", newValue, got)
	}

	// Delete
	err = db.Delete(key)
	if err != nil {
		t.Errorf("Delete failed: %v", err)
	}
	got, err = db.Get(key)
	if err == nil {
		t.Errorf("Expected error for deleted key, got value %s", got)
	}
}

func TestListAllKeysAndValues(t *testing.T) {
	db, err := NewDatabase()
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.db.Close()

	// 添加多个键值对
	db.Add("alpha", "1")
	db.Add("beta", "2")
	db.Add("gamma", "3")

	results, err := db.ListKeysAndValues()
	if err != nil {
		t.Errorf("ListKeysAndValues failed: %v", err)
	}

	if len(results) < 3 {
		t.Errorf("Expected at least 3 entries, got %d", len(results))
	}

	for k, v := range results {
		t.Logf("Key: %s, Value: %s", k, v)
	}
}
