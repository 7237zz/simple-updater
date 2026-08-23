package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DB struct {
	Host     string //
	Port     string //
	Username string //
	Password string //
	Database string //
	Engine   *gorm.DB
}

type Product struct {
	ID          int       `json:"id" gorm:"primaryKey;column:id;type:serial4"`
	CreatedTime time.Time `json:"created_time" gorm:"column:created_time;type:timestamp;default:CURRENT_TIMESTAMP(0)"` //创建时间
	Product     string    `json:"product" gorm:"column:product;type:varchar(255)"`
	System      string    `json:"system" gorm:"column:system;type:varchar(255)"`
	Version     string    `json:"version" gorm:"column:version;type:varchar(255)"`
	SHA256      string    `json:"sha256" gorm:"column:sha256;type:varchar(64);default:''"`
	URL         string    `json:"url" gorm:"column:url;type:varchar(255)"`
	Size        int64     `json:"size" gorm:"column:size;type:int4"`
	Files       []File    `json:"files" gorm:"column:files;type:jsonb;serializer:json"`
	FileName    string    `json:"file_name" gorm:"column:file_name;type:varchar(255)"`
	AppID       string    `json:"app_id" gorm:"column:app_id;type:varchar(255)"`
	UUID        string    `json:"uuid" gorm:"column:uuid;type:varchar(255)"`
	Data        *os.File  `json:"-" gorm:"-"`
	Bytes       []byte    `json:"-" gorm:"-"`
}

func initEngine(db *DB) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable", db.Host, db.Username, db.Password, db.Database, db.Port)
	engine, err := gorm.Open(postgres.Open(dsn), &gorm.Config{PrepareStmt: true, Logger: logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logger.Error,
			IgnoreRecordNotFoundError: true,
			Colorful:                  true,
		},
	)})
	if err != nil {
		return nil, err
	}
	err = engine.AutoMigrate(&Product{})
	if err != nil {
		return nil, err
	}
	return engine, nil
}

func (c *Client) uploadProduct2DB(product Product) error {
	return c.Engine.Create(&product).Error
}

func (c *Client) getLatestProduct(system string, appID string) (Product, error) {
	var product Product
	err := c.Engine.Where("system = ? AND app_id = ?", system, appID).Order("created_time desc").First(&product).Error
	return product, err
}
