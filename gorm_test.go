package regius

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type gormTestModel struct {
	ID   uint   `gorm:"primaryKey"`
	Name string `gorm:"size:255"`
}

func (gormTestModel) TableName() string {
	return "gorm_test_models"
}

func TestRegius_GORM(t *testing.T) {
	db := openSQLiteDB(t)
	r := &Regius{
		DB: Database{DataType: "sqlite", Pool: db},
	}

	gormDB, err := r.GORM()
	require.NoError(t, err)
	require.NotNil(t, gormDB)

	require.NoError(t, gormDB.AutoMigrate(&gormTestModel{}))

	result := gormDB.Create(&gormTestModel{Name: "hello"})
	require.NoError(t, result.Error)

	var found gormTestModel
	require.NoError(t, gormDB.First(&found, "name = ?", "hello").Error)
	assert.Equal(t, "hello", found.Name)
}

func TestRegius_GORM_NilPool(t *testing.T) {
	r := &Regius{DB: Database{DataType: "sqlite"}}

	gormDB, err := r.GORM()

	require.Error(t, err)
	assert.Nil(t, gormDB)
	assert.Contains(t, err.Error(), "pool is nil")
}

func TestRegius_GORM_UnsupportedType(t *testing.T) {
	db := openSQLiteDB(t)
	r := &Regius{DB: Database{DataType: "oracle", Pool: db}}

	gormDB, err := r.GORM()

	require.Error(t, err)
	assert.Nil(t, gormDB)
	assert.Contains(t, err.Error(), "unsupported database type")
}

func TestRegius_GORMWithConfig(t *testing.T) {
	db := openSQLiteDB(t)
	r := &Regius{
		DB: Database{DataType: "sqlite", Pool: db},
	}

	gormDB, err := r.GORMWithConfig(&gorm.Config{})
	require.NoError(t, err)
	require.NotNil(t, gormDB)
}

func TestRegius_AutoMigrate(t *testing.T) {
	db := openSQLiteDB(t)
	r := &Regius{
		DB: Database{DataType: "sqlite", Pool: db},
	}

	require.NoError(t, r.AutoMigrate(&gormTestModel{}))

	var count int64
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM gorm_test_models").Scan(&count))
	assert.Equal(t, int64(0), count)
}
