package main

import (
	"encoding/json"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"log"
	"os"
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Panic("%s: %s", msg, err)
	}
}

type Report struct {
	ReportId                         string `json:"ReportId"`
	CustomersAttendedInMorningShift  int    `json:"CustomersAttendedInMorningShift"`
	CustomerAttendedInAfternoonShift int    `json:"CustomerAttendedInAfternoonShift"`
	CustomerAttendedCount            int    `json:"CustomerAttendedCount"`
	Message                          string `json:"Message"`
}

func main() {
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	failOnError(err, "Error al conectar RabbitMQ")
	defer conn.Close()

	ch, err := conn.Channel()
	failOnError(err, "Error al abrir canal")
	defer ch.Close()

	q, err := ch.QueueDeclare("reportes", false, false, false, false, nil)
	failOnError(err, "Error al declarar cola")

	msgs, err := ch.Consume(q.Name, "", true, false, false, false, nil)
	failOnError(err, "Error al registrar el consumidor")
	os.Mkdir("reports/", 0644)

	var forever chan struct{}

	go func() {
		for d := range msgs {
			reportData := Report{}
			err := json.Unmarshal(d.Body, &reportData)
			failOnError(err, "Error al recibir mensaje")
			log.Println(reportData)

			f, err := os.OpenFile(fmt.Sprintf("reports/%s.csv", reportData.ReportId),
				os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				log.Println(err)
			}
			defer f.Close()
			t := fmt.Sprintf("%d,%d,%d,%s \n", reportData.CustomerAttendedCount, reportData.CustomersAttendedInMorningShift, reportData.CustomerAttendedInAfternoonShift, reportData.Message)
			if _, err := f.Write([]byte(t)); err != nil {
				log.Println(err)
			}
			/*fileContent, err := os.ReadFile(fmt.Sprintf("/reports/%s", reportData.ReportId))
			if err != nil {
				os.WriteFile(fmt.Sprintf("/reports/%s", reportData.ReportId), d.Body, 0644)
			} else {
				fmt.Println("antes", fileContent)
			}*/
			//log.Printf("%s dice: %s", messageData.Sender, messageData.MessageBody)
		}
	}()

	log.Printf("Esperando nuevos mensajes en cola: %s", q.Name)
	<-forever
}
