package model

import "time"

type Timestamp time.Time

type Attachements struct {
	Reference int
	Status int
	StatusDate Timestamp
	Path string
}

func (Attachements) TableName() string {
	return "Attachements"
}

type Schema struct {
	ConnString string
	Name string
	Dialect string
}
