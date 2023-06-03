package repo

import (
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	dsn = "host=43.154.135.12 user=postgres password=mypassword dbname=db port=18080 sslmode=disable TimeZone=Asia/Shanghai"
)

var Db *gorm.DB

func init() {
	log.Println("INIT Database...")

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic(err.Error())
	}

	Db = db

	// 迁移 schema
	err1 := Db.AutoMigrate(
		&Workspace{},
		&WorkspaceStats{},
	)
	if err1 != nil {
		panic(err.Error())
	}

	// just for test
	myTestDb()
}

func myTestDb() {
	log.Println("TEST Database...")

	var workspaces []Workspace

	log.Println("get workspace list:")
	Db.Limit(3).Offset(0).Find(&workspaces)
	for i, w := range workspaces {
		log.Printf("%dth => %v", i, w)
	}
	log.Println("get workspace list end")
}
