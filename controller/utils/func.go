package utils

import (
	"controller/config"
	"encoding/json"
	"fmt"
	"github.com/mitchellh/mapstructure"
	"github.com/streadway/amqp"
	"github.com/yossefazoulay/go_utils/queue"
	globalUtils "github.com/yossefazoulay/go_utils/utils"
	"os"
	"time"
)
func HandleError(err error, msg string, exit bool) {
	if err != nil {
		config.Logger.Log.Error(fmt.Sprintf("%s: %s", msg, err))
	}
	if exit {
		os.Exit(1)
	}
}

func MessageReceiver(m amqp.Delivery, rmq *queue.Rabbitmq)  {
	switch m.Headers["From"] {
	case "Transformer":
		getMessageFromTransformer(m, rmq)
	case "Worker" :
		GetMessageFromWorker(m, rmq)
	case "DAL":
		PoolReceiver(m, rmq)
	default:
		config.Logger.Log.Error("Received a message from a not known channel :", m.Headers["From"])
	}
}

func unpackFileMessage(m amqp.Delivery) *globalUtils.PickFile{
	log := config.Logger.Log
	pFIle := &globalUtils.PickFile{}
	err := json.Unmarshal(m.Body, pFIle)
	HandleError(err, "Unable to convert message to json", false)
	if err := m.Ack(false); err != nil {
		log.Error("Error acknowledging message : %s", err)
	}
	return pFIle
}

func getMessageFromTransformer(m amqp.Delivery, rmq *queue.Rabbitmq) {
	pFIle := unpackFileMessage(m)
	log:= config.Logger.Log
	if pFIle.Result["Transform"] == 1 {
		pFIle.Result = map[string]int{
			"BorderExist" : 0,
			"InsideJer" : 0,
		}

		mess, err := json.Marshal(pFIle)
		HandleError(err, "cannot convert transformed pFile to Json", false)
		res, err1 := rmq.SendMessage(mess, Constant.Channels.CheckDWG,  Constant.Headers["CheckDWG"])
		HandleError(err1, "message sending error", false)
		config.Logger.Log.Info(res)
	} else if pFIle.Result["Transform"] == 0 {
		log.Error("The transformer did not sucess to transform this file : " , pFIle.Path)
	}
}

func CheckResultsFromWorker(pFile *globalUtils.PickFile) int {
	for _, val := range pFile.Result {
		if globalUtils.Itob(val) {
			return 20
		}
	}
	return 10
}

func CreateErrorsInDB(pFile *globalUtils.PickFile) []byte {
	errorsMap := map[string]interface{}{}
	err := mapstructure.Decode(pFile.Result, &errorsMap)
	HandleError(err, "cannot decode error object to ORMKeyval", false)
	mess, err1 := json.Marshal(globalUtils.DbQuery{
		DbType: Constant.DBType,
		Schema:Constant.Schema,
		Table:  Constant.Cad_errors_table,
		CrudT:  Constant.CRUD.CREATE,
		Id: map[string]interface{}{
			"check_status_id" : pFile.Id,
		},
		ORMKeyVal: errorsMap,
	})
	if err1 != nil {
		HandleError(err1, "cannot create an update error object from worker message", false)
	}
	return mess
}

func sendMessageToQueue(body []byte, queueName string, headers map[string]interface{}, rmq *queue.Rabbitmq) {
	message, err := rmq.SendMessage(body,queueName , headers)
	if err != nil {
		config.Logger.Log.Error(err)
	} else {
		config.Logger.Log.Info("SEND : " + message)
	}
}


func GetMessageFromWorker(m amqp.Delivery, rmq *queue.Rabbitmq) {
	pFIle := unpackFileMessage(m)
	status := CheckResultsFromWorker(pFIle)
	if pFIle.Status != status || status == 20 {
		if status == 20 {
			mess := CreateErrorsInDB(pFIle)
			sendMessageToQueue(mess, Constant.Channels.Dal_Req, Constant.Headers["Dal_Req"], rmq)
		}
		mess, err := json.Marshal(globalUtils.DbQuery{
			DbType: Constant.DBType,
			Schema: Constant.Schema,
			Table:  Constant.Cad_check_table,
			CrudT:  Constant.CRUD.UPDATE,
			Id: map[string]interface{}{
				"Id" : pFIle.Id,
			},
			ORMKeyVal: map[string]interface{}{
				"status_code" : status,
			},
		})
		if err != nil {
			HandleError(err, "cannot create an update object from worker message", false)
		}
		sendMessageToQueue(mess, Constant.Channels.Dal_Req, Constant.Headers["Dal_Req"], rmq)
	}



}

func PoolReceiver(m amqp.Delivery, rmq *queue.Rabbitmq) {
	switch m.Headers["Type"] {
	case "retrieve":
		getRetrieveResponse(m, rmq)
	case "update":
		getUpdateResponse(m)
	}
}

func getUpdateResponse(m amqp.Delivery){
	config.Logger.Log.Info(string(m.Body))
}

func getRetrieveResponse(m amqp.Delivery, rmq *queue.Rabbitmq){
	type Timestamp time.Time
	type Attachements struct {
		Reference int
		Status int
		StatusDate Timestamp
		Path string
	}

	var res []Attachements
	err := json.Unmarshal(m.Body, &res)
	if err != nil {
		fmt.Println(err)
		config.Logger.Log.Error(err)
		HandleError(err, "MUST DISPATCH from POOL RECEIVER", false)
		config.Logger.Log.Error(string(m.Body))
	}
	for _, file := range res {
		message, err := json.Marshal(globalUtils.PickFile{
			Id: file.Reference,
			Path: file.Path,
			Status: file.Status,
			Result : map[string]int{
				"Transform" : 0,
			},
		})
		HandleError(err, "Cannot encode JSON", false)
		time.Sleep(time.Microsecond)
		res, err1 := rmq.SendMessage(message, Constant.Channels.ConvertDWG, Constant.Headers["ConvertDWG"])
		HandleError(err1, "message sending error", false)
		config.Logger.Log.Info(res)
	}

}

func Pooling(rmqConn *queue.Rabbitmq) {
	mess, _ := json.Marshal(globalUtils.DbQuery{
		DbType: Constant.DBType,
		Schema:Constant.Schema,
		Table:  Constant.Cad_check_table,
		CrudT:  Constant.CRUD.RETRIEVE,
		Id: map[string]interface{}{},
		ORMKeyVal: map[string]interface{}{
			"status" : 0,
		},
	})
	rmqConn.SendMessage(mess, Constant.Channels.Dal_Req, Constant.Headers["Dal_Req"])
}

func Scheduler(tick *time.Ticker, done chan bool, rmqConn *queue.Rabbitmq) {
	task(rmqConn, time.Now())
	for {
		select {
		case t := <-tick.C:
			task(rmqConn, t)
		case <-done:
			return
		}
	}
}

func task(rmqConn *queue.Rabbitmq, t time.Time) {
	Pooling(rmqConn)
}
