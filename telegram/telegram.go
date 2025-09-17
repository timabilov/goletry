package telegram

import (
	"fmt"
	"log"
	"os"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

var usernames string = os.Getenv("TG_ADMINS") //separated by comma from env

func EscapeMessage(message string) string {
	r := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"`", "\\`",
	)
	return r.Replace(message)
}

func RunWordBot(e *echo.Echo, db *gorm.DB) {

	if usernames == "" {
		usernames = "formality8765"
	}
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TG_TOKEN"))
	if err != nil {
		println("Error tg bot init")
		log.Panic(err)
	}
	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		fmt.Println(update)
		// password := ""
		// if len(usernames) > 4 && test.Contains(strings.Split(usernames, ","), update.FromChat().UserName) {
		// 	password = os.Getenv("ROOT_PASSWORD")
		// }

		// if update.Message != nil {
		// 	log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)
		// 	if update.Message.Command() == "start" {

		// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Söz siyahısına yeni söz əlavə etmək üçün dil və sözü  boşluq ilə ayırıb göndərin. Sözlər yoxlamadan keçir. \nDəstək olunan dillər: az, en, tr. \nMəsələn:\n`az hakim xalça yalan ...`\n`en pupil plan ...`")
		// 		msg.ParseMode = "markdown"
		// 		bot.Send(msg)
		// 		continue
		// 	} else if update.Message.Command() == "moderate" {
		// 		payload := test.InternalRequestJSON(e, "GET", "/config/admin/unvalidated", nil, password)

		// 		tg_buttons := [][]tgbotapi.InlineKeyboardButton{}
		// 		var r map[string][]models.Word
		// 		json.Unmarshal(payload, &r)
		// 		description := strings.Builder{}
		// 		description.WriteString("```\n")
		// 		for _, word := range r["words"] {
		// 			tg_buttons = append(tg_buttons, []tgbotapi.InlineKeyboardButton{
		// 				tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%s %s", word.Language.Emoji(), word.Word), fmt.Sprintf("adjective,true,%v", word.ID)),
		// 				tgbotapi.NewInlineKeyboardButtonData("✅", fmt.Sprintf("validate,true,%v", word.ID)),
		// 				tgbotapi.NewInlineKeyboardButtonData("✘", fmt.Sprintf("validate,false,%v", word.ID)),
		// 			})
		// 			description.WriteString(fmt.Sprintf("%s %s     🕐 %s      \n", word.Language.Emoji(), word.Word, word.CreatedAt.Format("2006-01-02")))
		// 			addedBy := word.AddedBy
		// 			if addedBy == "" {
		// 				addedBy = "(?)"
		// 			}
		// 			description.WriteString(fmt.Sprintf(" 👤 %s   \n", addedBy))
		// 		}
		// 		description.WriteString("```\n/moderate")
		// 		var numericKeyboard = tgbotapi.NewInlineKeyboardMarkup(
		// 			tg_buttons...,
		// 		)
		// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Yoxlamağa söz yoxdur ✅")
		// 		if len(r["words"]) > 0 {
		// 			msg.ReplyMarkup = numericKeyboard

		// 			msg.Text = fmt.Sprintf("Sözləri moderasiyadan keçir: \n%s", description.String())
		// 			msg.ParseMode = "markdown"
		// 		}
		// 		bot.Send(msg)
		// 		continue
		// 	} else if update.Message.Command() == "requests" {
		// 		requestType := ""
		// 		if len(strings.Split(update.Message.Text, " ")) == 2 {

		// 			requestType = strings.Split(update.Message.Text, " ")[1]
		// 		} else {
		// 			requestType = "support"
		// 		}
		// 		payload := test.InternalRequestJSON(e, "GET", fmt.Sprintf("/config/admin/requests?request_type=%v", requestType), nil, password)

		// 		tg_buttons := [][]tgbotapi.InlineKeyboardButton{}
		// 		var r map[string][]models.WordReportOut
		// 		json.Unmarshal(payload, &r)
		// 		description := strings.Builder{}
		// 		description.WriteString("```\n")
		// 		for _, word := range r["words"] {
		// 			allowed := ""
		// 			if word.Allowed != nil && *word.Allowed {
		// 				allowed = "✅"
		// 			}
		// 			tg_buttons = append(tg_buttons, []tgbotapi.InlineKeyboardButton{
		// 				tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%s %s %s", allowed, word.Language.Emoji(), word.Word), fmt.Sprintf("adjective,true,%v", word.ID)),
		// 				tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%v ✅", word.RequestCount), fmt.Sprintf("validate,true,%v", word.ID)),
		// 				tgbotapi.NewInlineKeyboardButtonData("✘", fmt.Sprintf("validate,false,%v", word.ID)),
		// 			})
		// 			description.WriteString(fmt.Sprintf("%s %s     🕐 %s      \n", word.Language.Emoji(), word.Word, word.CreatedAt.Format("2006-01-02")))
		// 			addedBy := word.AddedBy
		// 			if addedBy == "" {
		// 				addedBy = "(?)"
		// 			}
		// 			description.WriteString(fmt.Sprintf(" 👤 %s   \n", addedBy))
		// 		}
		// 		description.WriteString("```\n/requests")
		// 		var numericKeyboard = tgbotapi.NewInlineKeyboardMarkup(
		// 			tg_buttons...,
		// 		)
		// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Əlavə söz yoxdur ✅")
		// 		if len(r["words"]) > 0 {
		// 			msg.ReplyMarkup = numericKeyboard

		// 			msg.Text = fmt.Sprintf("Ən çox %s olunan sözlər: \n%s", requestType, description.String())
		// 			msg.ParseMode = "markdown"
		// 		}
		// 		bot.Send(msg)
		// 		continue
		// 	}

		// 	// If the message was open, add a copy of our numeric keyboard.
		// 	log.Printf("Message: %s ", update.Message.Text)
		// 	re := regexp.MustCompile("\\s+")
		// 	input := re.Split(strings.Trim(update.Message.Text, " "), -1)
		// 	fmt.Printf("%v\n", input)
		// 	if len(input) < 2 {
		// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Səhv sorğu: '%s' \nYeni söz göndərmək üçün:\n `>dil söz`. \nDəstək olunan dillər: az, en, tr. \nMəsələn:\n`az hakim xalça yalan ...`\n`en pupil plan ...`", update.Message.Text))
		// 		msg.ParseMode = "markdown"
		// 		bot.Send(msg)
		// 	} else {
		// 		if len(input) > 11 {
		// 			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Maksimum 10 sözə icazə var.")
		// 			msg.ParseMode = "markdown"
		// 			bot.Send(msg)
		// 			continue
		// 		}

		// 		lang := input[0]
		// 		words := input[1:]

		// 		messageSum := strings.Builder{}
		// 		addedBy := ""
		// 		if update.SentFrom() != nil {
		// 			addedBy = fmt.Sprintf("@%s", update.SentFrom().UserName)
		// 		}
		// 		for _, word := range words {

		// 			req := models.WordIn{
		// 				Word:     word,
		// 				Language: models.Language(lang),
		// 				AddedBy:  addedBy,
		// 			}
		// 			apiMessage := test.InternalRequestMessage(e, "POST", "/config/word", req, password)
		// 			messageSum.WriteString(EscapeMessage(apiMessage))
		// 			messageSum.WriteString("\n")
		// 		}
		// 		msg := tgbotapi.NewMessage(update.Message.Chat.ID, messageSum.String())
		// 		msg.ReplyToMessageID = update.Message.MessageID
		// 		msg.ParseMode = "markdown"
		// 		bot.Send(msg)
		// 	}

		// } else if update.CallbackQuery != nil {
		// 	// Respond to the callback query, telling Telegram to show the user
		// 	// a message with the data received.
		// 	if !strings.Contains(update.CallbackQuery.Data, ",") {
		// 		callback := tgbotapi.NewCallback(update.CallbackQuery.ID, update.CallbackQuery.Data)
		// 		if _, err := bot.Request(callback); err != nil {
		// 			panic(err)
		// 		}
		// 	} else {
		// 		action := strings.Split(update.CallbackData(), ",")[0]

		// 		allow := strings.Split(update.CallbackData(), ",")[1]
		// 		wordId := strings.Split(update.CallbackData(), ",")[2]
		// 		boolValue, err := strconv.ParseBool(allow)
		// 		var callback tgbotapi.CallbackConfig
		// 		if err != nil {

		// 			log.Printf("Səhv")
		// 			log.Println(err.Error())
		// 			callback = tgbotapi.NewCallback(update.CallbackQuery.ID, "Səhv baş verdi...")
		// 		} else {

		// 			req := models.WordValidate{
		// 				Allow: boolValue,
		// 			}
		// 			url := ""
		// 			if action == "validate" {
		// 				url = "/config/admin/validate/%s"
		// 			} else if action == "adjective" {
		// 				url = "/config/admin/make_adjective/%s"
		// 			} else {
		// 				callback = tgbotapi.NewCallback(update.CallbackQuery.ID, update.CallbackData())
		// 				if _, err := bot.Request(callback); err != nil {
		// 					panic(err)
		// 				}
		// 				continue
		// 			}
		// 			apiMessage := test.InternalRequestMessage(e, "POST", fmt.Sprintf(url, wordId), req, password)
		// 			callback = tgbotapi.NewCallback(update.CallbackQuery.ID, apiMessage)
		// 			inline := *update.CallbackQuery.Message.ReplyMarkup

		// 			for _, row := range inline.InlineKeyboard {
		// 				if len(row) == 3 {
		// 					if action == "validate" {

		// 						if strings.Contains(*row[1].CallbackData, update.CallbackData()) {
		// 							row[1].Text = fmt.Sprintf("👉 %s", strings.Trim(row[1].Text, "👉 "))
		// 							row[2].Text = strings.Trim(row[2].Text, "👉 ")
		// 						}
		// 						if strings.Contains(*row[2].CallbackData, update.CallbackData()) {
		// 							row[2].Text = fmt.Sprintf("👉 %s", row[2].Text)
		// 							row[1].Text = strings.Trim(row[1].Text, "👉 ")
		// 						}
		// 					} else if action == "adjective" {
		// 						row[0].CallbackData = test.NewRefString(fmt.Sprintf("adjective,%s,%s", strconv.FormatBool(!boolValue), strings.Split(*row[0].CallbackData, ",")[2]))
		// 					}
		// 				}
		// 			}
		// 			log.Printf("%v", inline)
		// 			messageInstance := update.CallbackQuery.Message
		// 			msg := tgbotapi.NewEditMessageReplyMarkup(messageInstance.Chat.ID, messageInstance.MessageID, inline)

		// 			if _, err = bot.Send(msg); err != nil {
		// 				log.Println(err.Error())
		// 			}
		// 		}
		// 		if _, err := bot.Request(callback); err != nil {
		// 			panic(err)
		// 		}
		// 	}

		// 	continue
		// }

	}

}
