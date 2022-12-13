package main

import (
	"context"
	"encoding/json"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	sid "github.com/teris-io/shortid"
	"log"
	"math/rand"
	"time"
)

type Report struct {
	ReportId                         string
	CustomersAttendedInMorningShift  int
	CustomerAttendedInAfternoonShift int
	CustomerAttendedCount            int
	Message                          string
}

type Station struct {
	Number              int  // Número de estación
	QueueCount          int  // Tamaño de la cola
	Available           bool // Si la estación está abierta
	NextMinuteAvailable int  // Cuando terminará de atender a la persona actual
	Occupied            bool // Si la estación está atendiendo a alguien
}

var MINUTES_IN_A_DAY = 1440
var MINUTES_IN_AN_HOUR = 60

func failOnError(err error, msg string) {
	if err != nil {
		log.Panic("%s: %s", msg, err)
	}
}

func getRandomBool(frequency float32) bool {
	rand.Seed(time.Now().UnixNano())
	var randomNumber = rand.Intn(100)
	return float32(randomNumber) <= frequency*100
}

func setUpStations(stationsCount int, availableResources int) []Station {
	var stations []Station
	for i := 0; i < stationsCount; i++ {
		newStation := Station{i, 0, i < availableResources, -1, false}
		stations = append(stations, newStation)
	}
	return stations
}

func changeShift(stations []Station, availableResources int) []Station {
	length := len(stations)
	for i := 0; i < length; i++ {
		stations[i].Available = length-i-1 < availableResources
	}
	return stations
}

func main() {
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	failOnError(err, "Error al conectar con RabbitMQ")
	defer conn.Close()

	ch, err := conn.Channel()
	failOnError(err, "Error al abrir canal")
	defer ch.Close()

	q, err := ch.QueueDeclare(
		"reportes",
		false,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "Error al declarar cola")

	rand.Seed(time.Now().UnixNano())
	currentDayMinutesPassed := 0
	isMorningShift := true
	var daysRunning, stationsCount, resourceCount int

	// TODO: Add validtion for inputs
	fmt.Println("Ingrese el número de días que desea ejecutar la simulación")
	fmt.Scanln(&daysRunning)

	fmt.Println("Ingrese la cantidad de estaciones a utilizar")
	fmt.Scanln(&stationsCount)

	fmt.Println("Ingrese la cantidad de recursos a utilizar")
	fmt.Scanln(&resourceCount)

	reportId, err := sid.Generate()

	var stations []Station
	var dailyReport Report
	finalReport := Report{reportId, 0, 0, 0, "reporte final"}

	daysConvertedToMinutes := daysRunning * MINUTES_IN_A_DAY
	passedDays := 0

	var frequency float32

	var morningShiftResources, afternoonShiftResources int

	for i := 0; i < daysConvertedToMinutes; i++ {
		if currentDayMinutesPassed == 0 {
			isMorningShift = true
			morningShiftResources = rand.Intn(stationsCount) + 1
			afternoonShiftResources = resourceCount - morningShiftResources
			fmt.Println(fmt.Sprintf("para el día %d se habilitan %d recursos para la mañana y %d recursos para la tarde", passedDays+1, morningShiftResources, afternoonShiftResources))
			stations = setUpStations(stationsCount, morningShiftResources)
			dailyReport = Report{reportId, 0, 0, 0, fmt.Sprintf("reporte del día %d", passedDays+1)}
			frequency = 0.31
		} else if currentDayMinutesPassed+1 == MINUTES_IN_AN_HOUR*3 {
			frequency = 0.46
		} else if currentDayMinutesPassed+1 == MINUTES_IN_AN_HOUR*6 {
			frequency = 0.55
		} else if float32(currentDayMinutesPassed+1) == float32(MINUTES_IN_AN_HOUR)*7.5 {
			isMorningShift = false
			stations = changeShift(stations, afternoonShiftResources)
			frequency = 0.23
		} else if currentDayMinutesPassed+1 == MINUTES_IN_AN_HOUR*9 {
			frequency = 0.73
		}

		var newCustomer = false
		// Se deja de recibir personas 20 minutos antes del final del día
		if currentDayMinutesPassed < (MINUTES_IN_AN_HOUR*13)-20 {
			newCustomer = getRandomBool(frequency)
		}

		if newCustomer {
			preferredStation := stations[0]
			for j := 0; j < stationsCount; j++ {
				currentStation := stations[j]
				currentStationHasPreference := currentStation.QueueCount < preferredStation.QueueCount || (preferredStation.Occupied && !currentStation.Occupied)
				if currentStation.Available && (!preferredStation.Available || currentStationHasPreference) {
					preferredStation = currentStation
				}
			}

			if preferredStation.QueueCount > 0 || preferredStation.Occupied {
				stations[preferredStation.Number].QueueCount = preferredStation.QueueCount + 1
			} else {
				rand.Seed(time.Now().UnixNano())
				var attentionTime = rand.Intn(6) + 5

				stations[preferredStation.Number].Occupied = true
				stations[preferredStation.Number].NextMinuteAvailable = currentDayMinutesPassed + attentionTime
			}

		}

		for j := 0; j < stationsCount; j++ {
			currentStation := stations[j]
			// Debuggear estaciones
			//fmt.Println(currentStation)
			if currentDayMinutesPassed == currentStation.NextMinuteAvailable {
				dailyReport.CustomerAttendedCount++

				if isMorningShift {
					dailyReport.CustomersAttendedInMorningShift++
				} else {
					dailyReport.CustomerAttendedInAfternoonShift++
				}
				if currentStation.QueueCount > 0 {
					stations[currentStation.Number].QueueCount = currentStation.QueueCount - 1

					var attentionTime = rand.Intn(6) + 5

					stations[currentStation.Number].NextMinuteAvailable = currentDayMinutesPassed + attentionTime
				} else {
					stations[currentStation.Number].Occupied = false
				}
			}
		}
		// Separar prints por ciclo de minuto
		//fmt.Println("=======")

		if i > 0 && (i+1)%MINUTES_IN_A_DAY == 0 {
			passedDays += 1
			currentDayMinutesPassed = 0
			finalReport.CustomerAttendedInAfternoonShift += dailyReport.CustomerAttendedInAfternoonShift
			finalReport.CustomersAttendedInMorningShift += dailyReport.CustomersAttendedInMorningShift
			finalReport.CustomerAttendedCount += dailyReport.CustomerAttendedCount

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			body, err := json.Marshal(dailyReport)
			err = ch.PublishWithContext(ctx, "", q.Name, false, false, amqp.Publishing{ContentType: "application/json", Body: body})
			failOnError(err, "Error al enviar mensaje")
		} else {
			currentDayMinutesPassed += 1
		}
		// 10 milisegundos de pausa para mejorar aleatoriedad
		time.Sleep(10)
	}

	// Hubiera preferido agrupar esto en una función, pero tuve problemas con el tipado del ch
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	body, err := json.Marshal(finalReport)
	err = ch.PublishWithContext(ctx, "", q.Name, false, false, amqp.Publishing{ContentType: "application/json", Body: body})
	failOnError(err, "Error al enviar mensaje")
}
