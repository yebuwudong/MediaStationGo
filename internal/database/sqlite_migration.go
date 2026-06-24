package database

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// MigrateSQLiteToCurrentIfNeeded copies an existing SQLite database into
// PostgreSQL. Redis is not migrated because it is a rebuildable cache, not a
// source of truth.
const sqliteMigrationCompleteSettingKey = "database.sqlite_migration_complete"

func MigrateSQLiteToCurrentIfNeeded(cfg *config.Config, target *gorm.DB, log *zap.Logger) error {
	if cfg == nil || target == nil || target.Dialector == nil || target.Dialector.Name() != "postgres" {
		return nil
	}
	sqlitePath, err := sqliteMigrationSourcePath(cfg, log)
	if err != nil {
		return err
	}
	if sqlitePath == "" {
		return nil
	}
	if complete, err := sqliteMigrationMarkedComplete(target); err != nil {
		return err
	} else if complete {
		if log != nil {
			log.Info("skip sqlite to postgres migration: already completed")
		}
		return nil
	}

	src, err := openSQLiteMigrationSource(cfg, sqlitePath)
	if err != nil {
		return fmt.Errorf("open sqlite migration source: %w", err)
	}
	sqlDB, err := src.DB()
	if err == nil {
		defer sqlDB.Close()
	}

	started := time.Now()
	if err := resetBootstrapTargetBeforeSQLiteMigrationIfSafe(src, target, log); err != nil {
		return err
	}
	copied, err := copyModelTables(src, target, 500)
	if err != nil {
		return err
	}
	if err := markSQLiteMigrationComplete(target); err != nil {
		return err
	}
	if log != nil {
		log.Info("sqlite data migrated to postgres",
			zap.String("source", sqlitePath),
			zap.Int64("rows", copied),
			zap.Duration("duration", time.Since(started)))
	}
	return nil
}

func openSQLiteMigrationSource(cfg *config.Config, sqlitePath string) (*gorm.DB, error) {
	srcCfg := *cfg
	srcCfg.Database.Type = "sqlite"
	srcCfg.Database.DBPath = sqlitePath
	return gorm.Open(sqlite.Open(buildSQLiteDSN(&srcCfg)), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
}

func sqliteMigrationSourcePath(cfg *config.Config, log *zap.Logger) (string, error) {
	configured := strings.TrimSpace(cfg.Database.DBPath)
	if configured != "" {
		exists, err := regularFileExists(configured)
		if err != nil {
			return "", fmt.Errorf("stat sqlite migration source: %w", err)
		}
		if exists {
			return configured, nil
		}
	}

	fallback := filepath.Join(strings.TrimSpace(cfg.App.DataDir), "mediastation.db")
	if fallback == "" || sameCleanPath(configured, fallback) {
		return "", nil
	}
	exists, err := regularFileExists(fallback)
	if err != nil {
		return "", fmt.Errorf("stat default sqlite migration source: %w", err)
	}
	if !exists {
		return "", nil
	}
	if log != nil && configured != "" {
		log.Warn("configured sqlite migration source not found; using data-dir default",
			zap.String("configured", configured),
			zap.String("fallback", fallback))
	}
	return fallback, nil
}

func regularFileExists(path string) (bool, error) {
	if strings.TrimSpace(path) == "" {
		return false, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return !info.IsDir(), nil
}

func sameCleanPath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func resetBootstrapTargetBeforeSQLiteMigrationIfSafe(src, target *gorm.DB, log *zap.Logger) error {
	hasRows, err := sqliteSourceHasMigratableRows(src)
	if err != nil {
		return err
	}
	if !hasRows {
		return nil
	}
	bootstrapOnly, err := targetLooksLikeBootstrapOnly(target)
	if err != nil || !bootstrapOnly {
		return err
	}
	for i := len(model.AllModels()) - 1; i >= 0; i-- {
		m := model.AllModels()[i]
		if !target.Migrator().HasTable(m) {
			continue
		}
		if err := target.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(m).Error; err != nil {
			return fmt.Errorf("clear bootstrap target table %T: %w", m, err)
		}
	}
	if log != nil {
		log.Warn("cleared bootstrap postgres rows before sqlite migration")
	}
	return nil
}

func sqliteSourceHasMigratableRows(src *gorm.DB) (bool, error) {
	for _, table := range []string{"users", "libraries", "media", "settings"} {
		exists, err := sqliteTableExists(src, table)
		if err != nil {
			return false, err
		}
		if !exists {
			continue
		}
		var count int64
		if err := src.Raw("SELECT COUNT(1) FROM " + quoteIdent(table)).Scan(&count).Error; err != nil {
			return false, fmt.Errorf("count sqlite table %s: %w", table, err)
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

func targetLooksLikeBootstrapOnly(target *gorm.DB) (bool, error) {
	for _, m := range []any{
		&model.Library{},
		&model.Series{},
		&model.Media{},
		&model.PlaybackHistory{},
		&model.Favorite{},
		&model.Playlist{},
		&model.PlaylistItem{},
		&model.DownloadTask{},
		&model.Subscription{},
	} {
		if !target.Migrator().HasTable(m) {
			continue
		}
		var count int64
		if err := target.Unscoped().Model(m).Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return false, nil
		}
	}

	var userCount int64
	if !target.Migrator().HasTable(&model.User{}) {
		return true, nil
	}
	if err := target.Model(&model.User{}).Count(&userCount).Error; err != nil {
		return false, err
	}
	if userCount == 0 {
		return true, nil
	}
	if userCount != 1 {
		return false, nil
	}
	var user model.User
	if err := target.Unscoped().Where("username = ?", "admin").First(&user).Error; err != nil {
		return false, nil
	}
	return user.Role == "admin", nil
}

func sqliteMigrationMarkedComplete(db *gorm.DB) (bool, error) {
	var value string
	err := db.Raw("SELECT value FROM "+quoteIdent("settings")+" WHERE "+quoteIdent("key")+" = ?", sqliteMigrationCompleteSettingKey).Scan(&value).Error
	if err != nil {
		return false, fmt.Errorf("check sqlite migration marker: %w", err)
	}
	return strings.EqualFold(strings.TrimSpace(value), "true"), nil
}

func markSQLiteMigrationComplete(db *gorm.DB) error {
	now := time.Now()
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&model.Setting{
		Key:       sqliteMigrationCompleteSettingKey,
		Value:     "true",
		UpdatedAt: now,
	}).Error; err != nil {
		return fmt.Errorf("mark sqlite migration complete: %w", err)
	}
	return nil
}

func copyModelTables(src, target *gorm.DB, batchSize int) (int64, error) {
	if batchSize <= 0 {
		batchSize = 500
	}
	var copied int64
	for _, m := range model.AllModels() {
		table, err := modelTableName(src, m)
		if err != nil {
			return copied, err
		}
		primaryColumns, err := modelPrimaryColumns(src, m)
		if err != nil {
			return copied, fmt.Errorf("inspect model %T primary keys: %w", m, err)
		}
		exists, err := sqliteTableExists(src, table)
		if err != nil {
			return copied, err
		}
		if !exists {
			continue
		}
		var sourceCount int64
		if err := src.Raw("SELECT COUNT(1) FROM " + quoteIdent(table)).Scan(&sourceCount).Error; err != nil {
			return copied, fmt.Errorf("count sqlite table %s: %w", table, err)
		}
		if sourceCount == 0 {
			continue
		}
		var targetCount int64
		if err := target.Raw("SELECT COUNT(1) FROM " + quoteIdent(table)).Scan(&targetCount).Error; err != nil {
			return copied, fmt.Errorf("count target table %s: %w", table, err)
		}
		modelType := reflect.TypeOf(m)
		if modelType.Kind() != reflect.Ptr {
			return copied, fmt.Errorf("model %T is not a pointer", m)
		}
		sliceType := reflect.SliceOf(modelType.Elem())
		slicePtr := reflect.New(sliceType)
		if err := src.Unscoped().Find(slicePtr.Interface()).Error; err != nil {
			return copied, fmt.Errorf("read sqlite table %s: %w", table, err)
		}
		filtered := slicePtr.Elem()
		if targetCount > 0 {
			primaryKeySet, err := targetPrimaryKeySet(target, table, primaryColumns)
			if err != nil {
				return copied, err
			}
			filtered = filterRowsMissingInTarget(target, table, primaryColumns, filtered, primaryKeySet)
		}
		if filtered.Len() == 0 {
			continue
		}
		filteredPtr := reflect.New(filtered.Type())
		filteredPtr.Elem().Set(filtered)
		if err := target.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(filteredPtr.Interface(), batchSize).Error; err != nil {
			return copied, fmt.Errorf("copy sqlite table %s: %w", table, err)
		}
		copied += int64(filtered.Len())
	}
	return copied, nil
}

func modelPrimaryColumns(db *gorm.DB, m any) ([]string, error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(m); err != nil {
		return nil, err
	}
	var cols []string
	for _, field := range stmt.Schema.PrimaryFields {
		cols = append(cols, field.DBName)
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("no primary key columns")
	}
	return cols, nil
}

func targetPrimaryKeySet(target *gorm.DB, table string, primaryColumns []string) (map[string]struct{}, error) {
	if len(primaryColumns) != 1 {
		return nil, nil
	}
	var values []string
	if err := target.Raw("SELECT " + quoteIdent(primaryColumns[0]) + " FROM " + quoteIdent(table)).Scan(&values).Error; err != nil {
		return nil, fmt.Errorf("read target primary keys for table %s: %w", table, err)
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set, nil
}

func filterRowsMissingInTarget(target *gorm.DB, table string, primaryColumns []string, rows reflect.Value, primaryKeySet map[string]struct{}) reflect.Value {
	if rows.Kind() != reflect.Slice || rows.Len() == 0 || len(primaryColumns) == 0 {
		return rows
	}
	out := reflect.MakeSlice(rows.Type(), 0, rows.Len())
	for i := 0; i < rows.Len(); i++ {
		row := rows.Index(i)
		keys, ok := rowPrimaryKeys(row, primaryColumns)
		if !ok {
			out = reflect.Append(out, row)
			continue
		}
		if primaryKeySet != nil {
			if _, exists := primaryKeySet[fmt.Sprint(keys[primaryColumns[0]])]; !exists {
				out = reflect.Append(out, row)
			}
			continue
		}
		if !targetHasPrimaryKey(target, table, keys) {
			out = reflect.Append(out, row)
		}
	}
	return out
}

func rowPrimaryKeys(row reflect.Value, primaryColumns []string) (map[string]any, bool) {
	if row.Kind() == reflect.Pointer {
		if row.IsNil() {
			return nil, false
		}
		row = row.Elem()
	}
	if row.Kind() != reflect.Struct {
		return nil, false
	}
	keys := make(map[string]any, len(primaryColumns))
	for _, column := range primaryColumns {
		value, ok := fieldByDBName(row, column)
		if !ok || value.IsZero() {
			return nil, false
		}
		keys[column] = value.Interface()
	}
	return keys, true
}

func fieldByDBName(row reflect.Value, column string) (reflect.Value, bool) {
	rowType := row.Type()
	for i := 0; i < row.NumField(); i++ {
		fieldType := rowType.Field(i)
		field := row.Field(i)
		if fieldType.Anonymous {
			if value, ok := fieldByDBName(field, column); ok {
				return value, true
			}
		}
		if columnNameForStructField(fieldType) == column {
			if field.Kind() == reflect.Pointer && field.IsNil() {
				return reflect.Value{}, false
			}
			return field, field.CanInterface()
		}
	}
	return reflect.Value{}, false
}

func columnNameForStructField(field reflect.StructField) string {
	if field.PkgPath != "" && !field.Anonymous {
		return ""
	}
	tag := field.Tag.Get("gorm")
	settings := schema.ParseTagSetting(tag, ";")
	if column := settings["COLUMN"]; column != "" {
		return column
	}
	return schema.NamingStrategy{}.ColumnName("", field.Name)
}

func targetHasPrimaryKey(target *gorm.DB, table string, keys map[string]any) bool {
	where := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	for _, column := range sortedMapKeys(keys) {
		where = append(where, quoteIdent(column)+" = ?")
		args = append(args, keys[column])
	}
	var count int64
	err := target.Raw("SELECT COUNT(1) FROM "+quoteIdent(table)+" WHERE "+strings.Join(where, " AND "), args...).Scan(&count).Error
	return err == nil && count > 0
}

func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sqliteTableExists(db *gorm.DB, table string) (bool, error) {
	var count int64
	if err := db.Raw(`SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count).Error; err != nil {
		return false, fmt.Errorf("inspect sqlite table %s: %w", table, err)
	}
	return count > 0, nil
}

func modelTableName(db *gorm.DB, m any) (string, error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(m); err != nil {
		return "", err
	}
	return stmt.Schema.Table, nil
}

func quoteIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
