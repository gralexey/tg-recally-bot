package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	botToken   = os.Getenv("TIMER_BOT_TOKEN")
	dbHost     = os.Getenv("TIMER_DB_HOST")
	dbPassword = os.Getenv("TIMER_DB_PASSWORD")
)

type TimersUsers struct {
	Id       int
	ChatId   int
	Text     string
	Time     int64
	Interval int
}

type Chat struct {
	Id            int64
	Username      string
	Firstname     string
	Lastname      string
	Title         string
	Type          string
	MessagesCount int
}

func recordStats(tgMessage *tgbotapi.Message, db *gorm.DB) {
	chat := Chat{}

	id := tgMessage.Chat.ID

	res := db.Find(&chat, id)

	if res.RowsAffected > 0 {
		chat.MessagesCount += 1
		db.Save(&chat)

	} else {
		tgChat := tgMessage.Chat
		chat := Chat{Id: tgChat.ID,
			Username:      tgChat.UserName,
			Firstname:     tgChat.FirstName,
			Lastname:      tgChat.LastName,
			Title:         tgChat.Title,
			Type:          tgChat.Type,
			MessagesCount: 1}

		result := db.Create(&chat)

		if result.Error != nil { // suppose it duplicates
			db.Model(&chat).Updates(chat)
		}
	}
}

func schedule(text string, id int, db *gorm.DB) bool {
	now := time.Now().Add(time.Second * time.Duration(3)).Unix()
	item := TimersUsers{
		ChatId:   id,
		Text:     text,
		Time:     now,
		Interval: 1,
	}
	res := db.Create(&item)
	return (res.RowsAffected == 1)
}

func scheduleTimer(bot *tgbotapi.BotAPI, db *gorm.DB) {
	ticker := time.NewTicker(60 * time.Second)

	go func() {
		for {
			select {
			case <-ticker.C:
				getAllDueAndFire(bot, db)
			}
		}
	}()
}

func getAllDueAndFire(bot *tgbotapi.BotAPI, db *gorm.DB) {
	items := []TimersUsers{}

	nowPlus := time.Now().Add(time.Minute * 1).Unix()

	db.Where("time < ? and time != 0", nowPlus).Find(&items)

	for _, item := range items {
		msg := tgbotapi.NewMessage(int64(item.ChatId), item.Text)
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Cancel", fmt.Sprint(item.Id)),
			),
		)
		_, err := bot.Send(msg)
		if err == nil {
			nextInterval := nextInt(item.Interval)
			if nextInterval == -1 {
				item.Time = 0
			} else {
				item.Time = time.Now().Add(time.Minute * time.Duration(nextInterval)).Unix()
				item.Interval = nextInterval
			}
			db.Save(&item)
		}
	}
}

func nextInt(curInt int) int {
	switch curInt {
	case 0:
		return 1
	case 1:
		return 5
	case 5:
		return 20
	case 20:
		return 60
	case 60:
		return 8 * 60
	case 8 * 60:
		return 24 * 60
	case 24 * 60:
		return 48 * 60
	default:
		return -1
	}
}

func main() {
	if len(botToken) == 0 {
		panic("no bot token provided")
	}
	if len(dbHost) == 0 {
		panic("no database host provided")
	}

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Authorized on account %s\n", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, uerr := bot.GetUpdatesChan(u)
	if uerr != nil {
		panic(uerr)
	}

	dsn := "host=" + dbHost + " user=postgres password=" + dbPassword + " dbname=postgres sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	scheduleTimer(bot, db)

	fmt.Print("listening to updates...")
	for update := range updates {
		if update.CallbackQuery != nil {
			bot.AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, update.CallbackQuery.Data))

			data := update.CallbackQuery.Data

			i, _ := strconv.Atoi(data)

			text := "-"

			item := TimersUsers{}
			res := db.Find(&item, i)
			if res.RowsAffected == 1 {
				item.Time = 0
				db.Save(&item)
				text = "Canceled"
			} else {
				text = "Error"
			}

			msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, text)
			bot.Send(msg)

		} else if update.Message != nil {
			recordStats(update.Message, db)

			switch update.Message.Text {
			case "/start":
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Send me a text to memorize 😎")
				bot.Send(msg)
				fmt.Println("started")

			default:
				text := "Scheduled!"
				if !schedule(update.Message.Text, int(update.Message.Chat.ID), db) {
					text = "Error happened"
				}
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
				bot.Send(msg)
				fmt.Println("planned ", text)
			}
		}
	}
}
