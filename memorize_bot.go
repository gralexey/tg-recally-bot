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
	botToken        = os.Getenv("TIMER_BOT_TOKEN")
	dbHost          = os.Getenv("TIMER_DB_HOST")
	dbPassword      = os.Getenv("TIMER_DB_PASSWORD")
	initialInterval = 2
)

type TimersUsers struct {
	Id         int
	ChatId     int
	Text       string
	Time       int64
	Interval   int
	LastMemoId int
}

type TimersChat struct {
	Id            int64
	Username      string
	Firstname     string
	Lastname      string
	Title         string
	Type          string
	MessagesCount int
}

func recordStats(tgMessage *tgbotapi.Message, db *gorm.DB) {
	chat := TimersChat{}

	id := tgMessage.Chat.ID

	res := db.Find(&chat, id)

	if res.RowsAffected > 0 {
		chat.MessagesCount += 1
		db.Save(&chat)

	} else {
		tgChat := tgMessage.Chat
		chat := TimersChat{Id: tgChat.ID,
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

func setLastMemoId(lastMemoId int, id int, db *gorm.DB) {
	item := TimersUsers{}
	res := db.Find(&item, id)
	if res.RowsAffected == 1 {
		item.LastMemoId = lastMemoId
		db.Save(&item)
	}
}

func schedule(text string, chatId int, db *gorm.DB) int {
	now := time.Now().Add(time.Second * time.Duration(3)).Unix()
	item := TimersUsers{
		ChatId:   chatId,
		Text:     text,
		Time:     now,
		Interval: initialInterval,
	}
	res := db.Create(&item)
	if res.RowsAffected == 1 {
		return item.Id
	} else {
		return -1
	}
}

func scheduleTimer(bot *tgbotapi.BotAPI, db *gorm.DB) {
	ticker := time.NewTicker(60 * time.Second)
	done := make(chan bool)

	go func() {
		for {
			select {
			case <-done:
				return
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

		if item.LastMemoId > 0 {
			bot.DeleteMessage(tgbotapi.DeleteMessageConfig{ChatID: int64(item.ChatId), MessageID: item.LastMemoId})
		}

		sentMsg, err := bot.Send(msg)
		if err == nil {
			nextInterval := nextInt(item.Interval)
			if nextInterval == -1 {
				item.Time = 0
			} else {
				item.Time = time.Now().Add(time.Minute * time.Duration(nextInterval)).Unix()
				item.Interval = nextInterval
				item.LastMemoId = sentMsg.MessageID
			}
			db.Save(&item)
		}
	}
}

func nextInt(curInt int) int {
	hour := 60
	eightHours := 8 * hour
	day := 24 * hour
	twoDays := 2 * day
	week := 7 * day
	sixteenDays := 16 * day
	thitryFiveDays := 35 * day

	switch curInt {
	case 0:
		return initialInterval
	case initialInterval:
		return 5
	case 5:
		return 20
	case 20:
		return 60
	case 60:
		return eightHours
	case eightHours:
		return day
	case day:
		return twoDays
	case twoDays:
		return week
	case week:
		return sixteenDays
	case sixteenDays:
		return thitryFiveDays
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

	createTablesIfNeeeded(db)

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
				text = fmt.Sprintf("\"%s\" canceled", item.Text)
			} else {
				text = "Error"
			}

			if item.LastMemoId > 0 {
				bot.DeleteMessage(tgbotapi.DeleteMessageConfig{ChatID: int64(item.ChatId), MessageID: item.LastMemoId})
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
				continue

			default:
				text := fmt.Sprintf("\"%s\" scheduled!", update.Message.Text)
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
				scheduledId := schedule(update.Message.Text, int(update.Message.Chat.ID), db)
				if scheduledId < 0 {
					text = "Error happened"
				}

				msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
					tgbotapi.NewInlineKeyboardRow(
						tgbotapi.NewInlineKeyboardButtonData("Cancel", fmt.Sprint(scheduledId)),
					),
				)

				sentMessage, err := bot.Send(msg)
				if err == nil {
					setLastMemoId(sentMessage.MessageID, scheduledId, db)
				}
				fmt.Println("planned ", text)
				continue
			}
		}
	}
}

func createTablesIfNeeeded(db *gorm.DB) {
	res1 := db.Exec(`CREATE TABLE timers_users (
		id SERIAL PRIMARY KEY,
		text text,
		interval integer,
		time integer,
		chat_id integer,
		last_memo_id integer
	);`)

	if res1.Error != nil {
		fmt.Println(res1.Error)
	}

	res2 := db.Exec(`CREATE TABLE timers_chats (
		id integer PRIMARY KEY,
		username character varying(32),
		firstname character varying(32),
		lastname character varying(32),
		isbot boolean,
		messages_count integer,
		title text,
		type character varying(16)
	);`)

	if res2.Error != nil {
		fmt.Println(res2.Error)
	}
}
