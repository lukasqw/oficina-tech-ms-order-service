package database

import (
	"fmt"
	"log/slog"
	serviceOrderPersistence "oficina-tech/internal/modules/service_order/infra/persistence"
	"oficina-tech/internal/shared/infra/observability"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

// EnvReader interface para facilitar testes
type EnvReader interface {
	Getenv(key string) string
}

// OSEnvReader implementação padrão que usa os.Getenv
type OSEnvReader struct{}

func (r *OSEnvReader) Getenv(key string) string {
	return os.Getenv(key)
}

// DBOpener interface para facilitar testes de conexão
type DBOpener interface {
	Open(dialector gorm.Dialector, config *gorm.Config) (*gorm.DB, error)
}

// GormDBOpener implementação padrão que usa gorm.Open
type GormDBOpener struct{}

func (o *GormDBOpener) Open(dialector gorm.Dialector, config *gorm.Config) (*gorm.DB, error) {
	return gorm.Open(dialector, config)
}

// BuildDSN constrói a string de conexão do banco de dados
func BuildDSN(envReader EnvReader) string {
	sslmode := envReader.Getenv("DB_SSLMODE")
	if sslmode == "" {
		sslmode = "disable"
	}

	return fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		envReader.Getenv("DB_HOST"),
		envReader.Getenv("DB_USER"),
		envReader.Getenv("DB_PASSWORD"),
		envReader.Getenv("DB_NAME"),
		envReader.Getenv("DB_PORT"),
		sslmode,
	)
}

// GetModelsToMigrate retorna a lista de modelos para migração
func GetModelsToMigrate() []interface{} {
	return []interface{}{
		&serviceOrderPersistence.ServiceOrderModel{},
		&serviceOrderPersistence.ServiceOrderItemModel{},
		&serviceOrderPersistence.ServiceOrderHistoryModel{},
	}
}

// ConnectWithDependencies versão testável da função Connect
func ConnectWithDependencies(envReader EnvReader, dbOpener DBOpener) (*gorm.DB, error) {
	dsn := BuildDSN(envReader)

	db, err := dbOpener.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Register OTel plugin for DB tracing
	if err := db.Use(observability.NewOTelPlugin()); err != nil {
		return nil, fmt.Errorf("failed to register OTel plugin: %w", err)
	}

	// Auto migrate
	if err := db.AutoMigrate(GetModelsToMigrate()...); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return db, nil
}

func Connect() {
	envReader := &OSEnvReader{}
	dbOpener := &GormDBOpener{}

	db, err := ConnectWithDependencies(envReader, dbOpener)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}

	DB = db
	slog.Info("Database connected successfully")
}
