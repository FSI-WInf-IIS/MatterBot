// Copyright (c) 2016 Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package main

import (
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"time"

	"github.com/mattermost/mattermost-server/v5/model"
)

const (
	SAMPLE_NAME = "SSI Mattermost Bot"

	USER_EMAIL    = "ssi@fsi-winf.de"
	USER_PASSWORD = ";svCF^20rT"
	USER_NAME     = "ssibot"
	USER_FIRST    = "SSI"
	USER_LAST     = "Bot"

	TEAM_NAME         = "general"
	CHANNEL_LOG_NAME  = "ssi_bot_debug"
	MAIN_CHANNEL_NAME = "town-square"
)

var client *model.Client4
var webSocketClient *model.WebSocketClient

var botUser *model.User
var botTeam *model.Team
var debuggingChannel *model.Channel
var mainChannel *model.Channel

// Documentation for the Go driver can be found
// at https://godoc.org/github.com/mattermost/platform/model#Client
func main() {
	println(SAMPLE_NAME)

	SetupGracefulShutdown()

	client = model.NewAPIv4Client("https://mattermost.fsi-winf.de")

	// Lets test to see if the mattermost server is up and running
	MakeSureServerIsRunning()

	// lets attempt to login to the Mattermost server as the bot user
	// This will set the token required for all future calls
	// You can get this token with client.AuthToken
	LoginAsTheBotUser()

	// If the bot user doesn't have the correct information lets update his profile
	UpdateTheBotUserIfNeeded()

	// Lets find our bot team
	FindBotTeam()

	// This is an important step.  Lets make sure we use the botTeam
	// for all future web service requests that require a team.
	//client.SetTeamId(botTeam.Id)

	// Lets create a bot channel for logging debug messages into
	CreateBotDebuggingChannelIfNeeded()
	SendMsgToDebuggingChannel("_"+SAMPLE_NAME+" has **started** running_", "")

	if rchannel, resp := client.GetChannelByName(MAIN_CHANNEL_NAME, botTeam.Id, ""); resp.Error != nil {
		println("We failed to get the channels")
		PrintError(resp.Error)
	} else {
		mainChannel = rchannel
	}

	// Lets start listening to some channels via the websocket!
	webSocketClient, err := model.NewWebSocketClient4("ws://mattermost.fsi-winf.de", client.AuthToken)
	if err != nil {
		println("We failed to connect to the web socket")
		PrintError(err)
	}

	webSocketClient.Listen()

	go func() {
		for {
			select {
			case resp := <-webSocketClient.EventChannel:
				HandleWebSocketResponse(resp)
			}
		}
	}()

	// You can block forever with
	select {}
}

func MakeSureServerIsRunning() {
	if props, resp := client.GetOldClientConfig(""); resp.Error != nil {
		println("There was a problem pinging the Mattermost server.  Are you sure it's running?")
		PrintError(resp.Error)
		os.Exit(1)
	} else {
		println("Server detected and is running version " + props["Version"])
	}
}

func LoginAsTheBotUser() {
	if user, resp := client.Login(USER_EMAIL, USER_PASSWORD); resp.Error != nil {
		println("There was a problem logging into the Mattermost server.  Are you sure ran the setup steps from the README.md?")
		PrintError(resp.Error)
		os.Exit(1)
	} else {
		botUser = user
	}
}

func UpdateTheBotUserIfNeeded() {
	if botUser.FirstName != USER_FIRST || botUser.LastName != USER_LAST || botUser.Username != USER_NAME {
		botUser.FirstName = USER_FIRST
		botUser.LastName = USER_LAST
		botUser.Username = USER_NAME

		if user, resp := client.UpdateUser(botUser); resp.Error != nil {
			println("We failed to update the Sample Bot user")
			PrintError(resp.Error)
			os.Exit(1)
		} else {
			botUser = user
			println("Looks like this might be the first run so we've updated the bots account settings")
		}
	}
}

func FindBotTeam() {
	if team, resp := client.GetTeamByName(TEAM_NAME, ""); resp.Error != nil {
		println("We failed to get the initial load")
		println("or we do not appear to be a member of the team '" + TEAM_NAME + "'")
		PrintError(resp.Error)
		os.Exit(1)
	} else {
		botTeam = team
	}
}

func CreateBotDebuggingChannelIfNeeded() {
	if rchannel, resp := client.GetChannelByName(CHANNEL_LOG_NAME, botTeam.Id, ""); resp.Error != nil {
		println("We failed to get the channels")
		PrintError(resp.Error)
	} else {
		debuggingChannel = rchannel
		return
	}

	// Looks like we need to create the logging channel
	channel := &model.Channel{}
	channel.Name = CHANNEL_LOG_NAME
	channel.DisplayName = "Debugging For SSI Bot"
	channel.Purpose = "This is used as a test channel for logging bot debug messages"
	channel.Type = model.CHANNEL_OPEN
	channel.TeamId = botTeam.Id
	if rchannel, resp := client.CreateChannel(channel); resp.Error != nil {
		println("We failed to create the channel " + CHANNEL_LOG_NAME)
		PrintError(resp.Error)
	} else {
		debuggingChannel = rchannel
		println("Looks like this might be the first run so we've created the channel " + CHANNEL_LOG_NAME)
	}
}

func SendMsgToDebuggingChannel(msg string, replyToId string) {
	post := &model.Post{}
	post.ChannelId = debuggingChannel.Id
	post.Message = msg

	post.RootId = replyToId

	SendMsgToChannel(post)
}

func SendMsgToChannel(post *model.Post) {
	if _, resp := client.CreatePost(post); resp.Error != nil {
		println("We failed to send a message to the logging channel")
		PrintError(resp.Error)
	}
}

func HandleWebSocketResponse(event *model.WebSocketEvent) {
	// only answer to posted events
	if event.Event == model.WEBSOCKET_EVENT_TYPING {
		if event.Data["user_id"] == "qa8frsba7fd4mji4nt39pjtsmc" && event.Broadcast.ChannelId == debuggingChannel.Id {
			SendMsgToDebuggingChannel("Max, hör auf zu schreiben!", "")
		}
		return
	}

	if event.Event == model.WEBSOCKET_EVENT_NEW_USER {
		HandleNewUser(event)
	}

	if event.Event == model.WEBSOCKET_EVENT_POSTED {
		// If this isn't the debugging channel then lets ingore it
		if event.Broadcast.ChannelId == debuggingChannel.Id {
			HandleMsgFromDebuggingChannel(event)
		} else if event.Broadcast.ChannelId == mainChannel.Id {
			HandleMsgFromMainChannel(event)
		}
	}
}

func HandleMsgFromDebuggingChannel(event *model.WebSocketEvent) {
	println("responding to debugging channel msg")
	post := model.PostFromJson(strings.NewReader(event.Data["post"].(string)))
	if post != nil {

		// ignore my events
		if post.UserId == botUser.Id {
			return
		}

		// if you see any word matching 'alive' then respond
		if matched, _ := regexp.MatchString(`(?:^|\W)alive(?:$|\W)`, post.Message); matched {
			SendMsgToDebuggingChannel("Yes I'm running", post.Id)
			return
		}

		// if you see any word matching 'up' then respond
		if matched, _ := regexp.MatchString(`(?:^|\W)up(?:$|\W)`, post.Message); matched {
			SendMsgToDebuggingChannel("Yes I'm running", post.Id)
			return
		}

		// if you see any word matching 'running' then respond
		if matched, _ := regexp.MatchString(`(?:^|\W)running(?:$|\W)`, post.Message); matched {
			SendMsgToDebuggingChannel("Yes I'm running", post.Id)
			return
		}

		// if you see any word matching 'hello' then respond
		if matched, _ := regexp.MatchString(`(?:^|\W)hello(?:$|\W)`, post.Message); matched {
			SendMsgToDebuggingChannel("Yes I'm running", post.Id)
			return
		}
		// if you see any word matching 'hello' then respond
		if matched, _ := regexp.MatchString(`(?:^|\W)geilste(?:$|\W)`, post.Message); matched {
			SendMsgToDebuggingChannel("Eindeutig Fabian!", post.Id)
			return
		}
	}

	SendMsgToDebuggingChannel("I did not understand you!", post.Id)
}

func HandleMsgFromMainChannel(event *model.WebSocketEvent) {
	println("responding to main channel msg")

	post := model.PostFromJson(strings.NewReader(event.Data["post"].(string)))
	if post != nil {
		client.DeletePost(post.Id)
	}
}

func HandleNewUser(event *model.WebSocketEvent) {
	println("new user!")

	userId := event.Data["user_id"].(string)
	channel, response := client.CreateDirectChannel(botUser.Id, userId)
	if response.StatusCode != 201 {
		SendMsgToDebuggingChannel(fmt.Sprintf("failed to establish private channel to %v", "user.Nickname"), "")
		return
	}
	user, resp := client.GetUser(userId, "")
	if resp.StatusCode != 200 {
		fmt.Printf("failed getting user for id %v", userId)
	}
	post := &model.Post{}
	post.ChannelId = channel.Id
	post.Message = fmt.Sprintf("Hallo %v!", user.Username)
	client.CreatePost(post)
	time.Sleep(1 * time.Second)
	post.Message = "willkommen bei der “Student Socialization Initiative against COVID-19” (SSI), einer Initiative der " +
		"FSI WInf/IIS der FAU und der FS WIAI der OFU."
	client.CreatePost(post)
	time.Sleep(1 * time.Second)
	post.Message = "Die Idee hinter dieser Initiative ist:\n" +
		"- Lernaustausch\n" +
		"- Diskussionsmöglichkeiten\n" +
		"- Initiativen-Koordination Online\n" +
		"- trotz Social Distancing neue Kontakte knüpfen in einem zielorientierten Umfeld für Studierende aus " +
		"ganz Deutschland"
	client.CreatePost(post)
	time.Sleep(1 * time.Second)
	post.Message = "\"_main\": Hier findet ihr eine stets aktuelle Übersicht der Initiative\n" +
		"\"_news\": Neuigkeiten zu SSI und neue Lern- und Austauschmöglichkeiten\n" +
		"\"_Lectures\": Wenn ihr einen Vortrag zu einem Thema halten wollt, dann könnt ihr das hier vorschlagen oder " +
		"einfach Vorträgen zuhören die hier stattfinden.\n" +
		"\"_Q&A\": Ihr habt ein Problem? Hier könnt ihr nach Lösungen fragen\n" +
		"\"_Topic Suggestions\": Hier könnt ihr neue Themenvorschläge einbringen. Für diese wird dann eine Umfrage " +
		"gestartet, und wenn sich genügend Leute finden, wird ein neuer Channel erstellt\n" +
		"\"_Town Square\": Ein Platz für den ganz offenen Austausch. Hier werden vermutlich die meisten Memes & co. " +
		"geteilt"
	client.CreatePost(post)
	time.Sleep(1 * time.Second)
	post.Message = "Mit welchen Themen willst du dich als Mitglied unserer Initiative auseinandersetzen:\n" +
		"- Machine Learning\n- Programmierung\n- CS-General\n- Zeig mir alles!"
	client.CreatePost(post)
}

func PrintError(err *model.AppError) {
	println("\tError Details:")
	println("\t\t" + err.Message)
	println("\t\t" + err.Id)
	println("\t\t" + err.DetailedError)
}

func SetupGracefulShutdown() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for _ = range c {
			if webSocketClient != nil {
				webSocketClient.Close()
			}

			SendMsgToDebuggingChannel("_"+SAMPLE_NAME+" has **stopped** running_", "")
			os.Exit(0)
		}
	}()
}
