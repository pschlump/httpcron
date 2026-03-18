package config

import (
	"strings"
	"testing"
)

func TestSetDefaults_StringFields(t *testing.T) {
	type Config struct {
		Name    string `default:"TestApp"`
		Version string `default:"1.0.0"`
		NoTag   string
		EnvTest string `default:"$ENV$zABz912839012038129038012890312983812"`
	}

	config := &Config{}
	err := SetDefaults(config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.Name != "TestApp" {
		t.Errorf("expected Name='TestApp', got '%s'", config.Name)
	}

	if config.Version != "1.0.0" {
		t.Errorf("expected Version='1.0.0', got '%s'", config.Version)
	}

	if config.EnvTest != "" {
		t.Errorf("expected EnvTest='', got '%s'", config.EnvTest)
	}

	if config.NoTag != "" {
		t.Errorf("expected NoTag='', got '%s'", config.NoTag)
	}
}

func TestSetDefaults_IntFields(t *testing.T) {
	type Config struct {
		Port      int   `default:"8080"`
		Timeout   int32 `default:"30"`
		MaxConns  int64 `default:"1000"`
		SmallInt  int8  `default:"127"`
		MediumInt int16 `default:"32000"`
	}

	config := &Config{}
	err := SetDefaults(config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.Port != 8080 {
		t.Errorf("expected Port=8080, got %d", config.Port)
	}

	if config.Timeout != 30 {
		t.Errorf("expected Timeout=30, got %d", config.Timeout)
	}

	if config.MaxConns != 1000 {
		t.Errorf("expected MaxConns=1000, got %d", config.MaxConns)
	}

	if config.SmallInt != 127 {
		t.Errorf("expected SmallInt=127, got %d", config.SmallInt)
	}

	if config.MediumInt != 32000 {
		t.Errorf("expected MediumInt=32000, got %d", config.MediumInt)
	}
}

func TestSetDefaults_UintFields(t *testing.T) {
	type Config struct {
		BufferSize uint   `default:"1024"`
		MaxRetries uint32 `default:"3"`
		BigNumber  uint64 `default:"9999999999"`
		TinyUint   uint8  `default:"255"`
		SmallUint  uint16 `default:"65000"`
	}

	config := &Config{}
	err := SetDefaults(config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.BufferSize != 1024 {
		t.Errorf("expected BufferSize=1024, got %d", config.BufferSize)
	}

	if config.MaxRetries != 3 {
		t.Errorf("expected MaxRetries=3, got %d", config.MaxRetries)
	}

	if config.BigNumber != 9999999999 {
		t.Errorf("expected BigNumber=9999999999, got %d", config.BigNumber)
	}

	if config.TinyUint != 255 {
		t.Errorf("expected TinyUint=255, got %d", config.TinyUint)
	}

	if config.SmallUint != 65000 {
		t.Errorf("expected SmallUint=65000, got %d", config.SmallUint)
	}
}

func TestSetDefaults_FloatFields(t *testing.T) {
	type Config struct {
		Rate      float32 `default:"3.14"`
		Precision float64 `default:"2.71828"`
	}

	config := &Config{}
	err := SetDefaults(config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.Rate != 3.14 {
		t.Errorf("expected Rate=3.14, got %f", config.Rate)
	}

	if config.Precision != 2.71828 {
		t.Errorf("expected Precision=2.71828, got %f", config.Precision)
	}
}

func TestSetDefaults_BoolFields(t *testing.T) {
	type Config struct {
		Debug   bool `default:"true"`
		Verbose bool `default:"false"`
		NoTag   bool
	}

	config := &Config{}
	err := SetDefaults(config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !config.Debug {
		t.Errorf("expected Debug=true, got false")
	}

	if config.Verbose {
		t.Errorf("expected Verbose=false, got true")
	}

	if config.NoTag {
		t.Errorf("expected NoTag=false, got true")
	}
}

func TestSetDefaults_NestedStruct(t *testing.T) {
	type Database struct {
		Host string `default:"localhost"`
		Port int    `default:"5432"`
	}

	type Config struct {
		AppName  string `default:"MyApp"`
		Database Database
	}

	config := &Config{}
	err := SetDefaults(config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.AppName != "MyApp" {
		t.Errorf("expected AppName='MyApp', got '%s'", config.AppName)
	}

	if config.Database.Host != "localhost" {
		t.Errorf("expected Database.Host='localhost', got '%s'", config.Database.Host)
	}

	if config.Database.Port != 5432 {
		t.Errorf("expected Database.Port=5432, got %d", config.Database.Port)
	}
}

func TestSetDefaults_PointerToStruct(t *testing.T) {
	type Address struct {
		City string `default:"New York"`
		Zip  int    `default:"10001"`
	}

	type Config struct {
		Name    string `default:"Test"`
		Address *Address
	}

	config := &Config{}
	err := SetDefaults(config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.Name != "Test" {
		t.Errorf("expected Name='Test', got '%s'", config.Name)
	}

	if config.Address == nil {
		t.Fatal("expected Address to be initialized, got nil")
	}

	if config.Address.City != "New York" {
		t.Errorf("expected Address.City='New York', got '%s'", config.Address.City)
	}

	if config.Address.Zip != 10001 {
		t.Errorf("expected Address.Zip=10001, got %d", config.Address.Zip)
	}
}

func TestSetDefaults_PreexistingValues(t *testing.T) {
	type Config struct {
		Name string `default:"DefaultName"`
		Port int    `default:"8080"`
	}

	config := &Config{
		Name: "CustomName",
		Port: 9000,
	}
	err := SetDefaults(config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// SetDefaults should overwrite existing values
	if config.Name != "DefaultName" {
		t.Errorf("expected Name='DefaultName', got '%s'", config.Name)
	}

	if config.Port != 8080 {
		t.Errorf("expected Port=8080, got %d", config.Port)
	}
}

func TestSetDefaults_InvalidIntValue(t *testing.T) {
	type Config struct {
		Port int `default:"not_a_number"`
	}

	config := &Config{}
	err := SetDefaults(config)

	if err == nil {
		t.Fatal("expected error for invalid int value, got nil")
	}

	if !strings.Contains(err.Error(), "invalid int default value") {
		t.Errorf("expected error message about invalid int, got: %v", err)
	}
}

func TestSetDefaults_InvalidUintValue(t *testing.T) {
	type Config struct {
		Count uint `default:"-5"`
	}

	config := &Config{}
	err := SetDefaults(config)

	if err == nil {
		t.Fatal("expected error for negative uint value, got nil")
	}

	if !strings.Contains(err.Error(), "invalid uint default value") {
		t.Errorf("expected error message about invalid uint, got: %v", err)
	}
}

func TestSetDefaults_InvalidFloatValue(t *testing.T) {
	type Config struct {
		Rate float64 `default:"abc"`
	}

	config := &Config{}
	err := SetDefaults(config)

	if err == nil {
		t.Fatal("expected error for invalid float value, got nil")
	}

	if !strings.Contains(err.Error(), "invalid float default value") {
		t.Errorf("expected error message about invalid float, got: %v", err)
	}
}

func TestSetDefaults_InvalidBoolValue(t *testing.T) {
	type Config struct {
		Debug bool `default:"yes"`
	}

	config := &Config{}
	err := SetDefaults(config)

	if err == nil {
		t.Fatal("expected error for invalid bool value, got nil")
	}

	if !strings.Contains(err.Error(), "invalid bool default value") {
		t.Errorf("expected error message about invalid bool, got: %v", err)
	}
}

func TestSetDefaults_NotAPointer(t *testing.T) {
	type Config struct {
		Name string `default:"Test"`
	}

	config := Config{}
	err := SetDefaults(config)

	if err == nil {
		t.Fatal("expected error when passing non-pointer, got nil")
	}

	if !strings.Contains(err.Error(), "requires a pointer") {
		t.Errorf("expected error about pointer requirement, got: %v", err)
	}
}

func TestSetDefaults_PointerToNonStruct(t *testing.T) {
	name := "test"
	err := SetDefaults(&name)

	if err == nil {
		t.Fatal("expected error when passing pointer to non-struct, got nil")
	}

	if !strings.Contains(err.Error(), "requires a pointer to a struct") {
		t.Errorf("expected error about struct requirement, got: %v", err)
	}
}

func TestSetDefaults_UnexportedFields(t *testing.T) {
	type Config struct {
		Name         string `default:"Public"`
		unexported   string `default:"Private"`
		AnotherField int    `default:"42"`
	}

	config := &Config{}
	err := SetDefaults(config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.Name != "Public" {
		t.Errorf("expected Name='Public', got '%s'", config.Name)
	}

	// Unexported field should remain empty
	if config.unexported != "" {
		t.Errorf("expected unexported='', got '%s'", config.unexported)
	}

	if config.AnotherField != 42 {
		t.Errorf("expected AnotherField=42, got %d", config.AnotherField)
	}
}

func TestSetDefaults_DeepNesting(t *testing.T) {
	type Level3 struct {
		Value string `default:"level3"`
	}

	type Level2 struct {
		Value  string `default:"level2"`
		Level3 Level3
	}

	type Level1 struct {
		Value  string `default:"level1"`
		Level2 Level2
	}

	config := &Level1{}
	err := SetDefaults(config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.Value != "level1" {
		t.Errorf("expected Level1.Value='level1', got '%s'", config.Value)
	}

	if config.Level2.Value != "level2" {
		t.Errorf("expected Level2.Value='level2', got '%s'", config.Level2.Value)
	}

	if config.Level2.Level3.Value != "level3" {
		t.Errorf("expected Level3.Value='level3', got '%s'", config.Level2.Level3.Value)
	}
}

func TestSetDefaults_EmptyDefaultTag(t *testing.T) {
	type Config struct {
		Name  string `default:""`
		Count int    `default:""`
	}

	config := &Config{
		Name:  "Original",
		Count: 42,
	}
	err := SetDefaults(config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Empty default tags should be ignored, values unchanged
	if config.Name != "Original" {
		t.Errorf("expected Name='Original', got '%s'", config.Name)
	}

	if config.Count != 42 {
		t.Errorf("expected Count=42, got %d", config.Count)
	}
}

func TestSetDefaults_NegativeNumbers(t *testing.T) {
	type Config struct {
		Temperature int   `default:"-10"`
		Offset      int64 `default:"-999"`
	}

	config := &Config{}
	err := SetDefaults(config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.Temperature != -10 {
		t.Errorf("expected Temperature=-10, got %d", config.Temperature)
	}

	if config.Offset != -999 {
		t.Errorf("expected Offset=-999, got %d", config.Offset)
	}
}

func TestSetDefaults_ZeroValues(t *testing.T) {
	type Config struct {
		Count   int     `default:"0"`
		Rate    float64 `default:"0.0"`
		Enabled bool    `default:"false"`
		Name    string  `default:""`
	}

	config := &Config{
		Count:   100,
		Rate:    3.14,
		Enabled: true,
		Name:    "Test",
	}
	err := SetDefaults(config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.Count != 0 {
		t.Errorf("expected Count=0, got %d", config.Count)
	}

	if config.Rate != 0.0 {
		t.Errorf("expected Rate=0.0, got %f", config.Rate)
	}

	if config.Enabled {
		t.Errorf("expected Enabled=false, got true")
	}

	// Empty string default tag should be ignored
	if config.Name != "Test" {
		t.Errorf("expected Name='Test', got '%s'", config.Name)
	}
}

/* vim: set noai ts=4 sw=4: */
