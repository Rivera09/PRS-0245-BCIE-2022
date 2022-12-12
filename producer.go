package main

import (
	"context"
	"encoding/json"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"log"
	"math/rand"
	"time"
)

type Report struct {
	customersAttendedInMorningShift  int
	customerAttendedInAfternoonShift int
	customerAttendedCount            int
	message                          string
}

type Station struct {
	number              int  // Número de estación
	queueCount          int  // Tamaño de la cola
	available           bool // Si la estación está abierta
	nextMinuteAvailable int  // Cuando terminará de atender a la persona actual
	occupied            bool // Si la estación está atendiendo a alguien
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
		stations[i].available = length-i-1 < availableResources
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

	var stations []Station
	var dailyReport Report
	finalReport := Report{0, 0, 0, "reporte final"}

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
			dailyReport = Report{0, 0, 0, fmt.Sprintf("reporte del día %s", passedDays+1)}
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
				currentStationHasPreference := currentStation.queueCount < preferredStation.queueCount || (preferredStation.occupied && !currentStation.occupied)
				if currentStation.available && (!preferredStation.available || currentStationHasPreference) {
					preferredStation = currentStation
				}
			}

			if preferredStation.queueCount > 0 || preferredStation.occupied {
				stations[preferredStation.number].queueCount = preferredStation.queueCount + 1
			} else {
				rand.Seed(time.Now().UnixNano())
				var attentionTime = rand.Intn(6) + 5

				stations[preferredStation.number].occupied = true
				stations[preferredStation.number].nextMinuteAvailable = currentDayMinutesPassed + attentionTime
			}

		}

		for j := 0; j < stationsCount; j++ {
			currentStation := stations[j]
			// Debuggear estaciones
			//fmt.Println(currentStation)
			if currentDayMinutesPassed == currentStation.nextMinuteAvailable {
				dailyReport.customerAttendedCount++

				if isMorningShift {
					dailyReport.customersAttendedInMorningShift++
				} else {
					dailyReport.customerAttendedInAfternoonShift++
				}
				if currentStation.queueCount > 0 {
					stations[currentStation.number].queueCount = currentStation.queueCount - 1

					var attentionTime = rand.Intn(6) + 5

					stations[currentStation.number].nextMinuteAvailable = currentDayMinutesPassed + attentionTime
				} else {
					stations[currentStation.number].occupied = false
				}
			}
		}
		// Separar prints por ciclo de minuto
		//fmt.Println("=======")

		if i > 0 && (i+1)%MINUTES_IN_A_DAY == 0 {
			passedDays += 1
			currentDayMinutesPassed = 0
			finalReport.customerAttendedInAfternoonShift += dailyReport.customerAttendedInAfternoonShift
			finalReport.customersAttendedInMorningShift += dailyReport.customersAttendedInMorningShift
			finalReport.customerAttendedCount += dailyReport.customerAttendedCount

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
