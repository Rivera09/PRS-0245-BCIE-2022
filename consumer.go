package main

import (
	"encoding/json"
	amqp "github.com/rabbitmq/amqp091-go"
	"log"
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Panic("%s: %s", msg, err)
	}
}

type Report struct {
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

	var forever chan struct{}

	go func() {
		for d := range msgs {
			reportData := Report{}
			err := json.Unmarshal(d.Body, &reportData)
			failOnError(err, "Error al recibir mensaje")
			log.Println(reportData)
			//log.Printf("%s dice: %s", messageData.Sender, messageData.MessageBody)
		}
	}()

	log.Printf("Esperando nuevos mensajes en cola: %s", q.Name)
	<-forever
}
