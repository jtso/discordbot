package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

func init() {
	flag.StringVar(&token, "t", "", "Bot Token")
	flag.Parse()
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

var token string
var buffer = make([][]byte, 0)
var hamu_hunger_regexp = regexp.MustCompile("([iI]( am|'m) [hH]ungry|^[hH]ungry$)")
var bot_commands = make(map[string]string)
var authorised_users = make(map[string]string)

func main() {

	if token == "" {
		fmt.Println("No token provided. Please run: discordbot -t <bot token>")
		return
	}

	// Load the sound file.
	err := loadSound()
	if err != nil {
		fmt.Println("Error loading sound: ", err)
		fmt.Println("Please copy $GOPATH/src/github.com/bwmarrin/examples/airhorn/airhorn.dca to this directory.")
		return
	}

	// Create a new Discord session using the provided bot token.
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println("Error creating Discord session: ", err)
		return
	}

	dat, read_err := ioutil.ReadFile("./botcommands.json")
	check(read_err)
	if unmarshal_err := json.Unmarshal(dat, &bot_commands); unmarshal_err != nil {
		panic(unmarshal_err)
	}
	authorised_users["183017941158068226"] = "Alycaea"

	// Register ready as a callback for the ready events.
	dg.AddHandler(ready)

	// Register messageCreate as a callback for the messageCreate events.
	dg.AddHandler(messageCreate)

	// Register guildCreate as a callback for the guildCreate events.
	dg.AddHandler(guildCreate)

	// Open the websocket and begin listening.
	err = dg.Open()
	if err != nil {
		fmt.Println("Error opening Discord session: ", err)
	}

	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("Alybot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	dg.Close()
}

// This function will be called (due to AddHandler above) when the bot receives
// the "ready" event from Discord.
func ready(s *discordgo.Session, event *discordgo.Ready) {

	// Set the playing status.
	s.UpdateStatus(0, "with Alycaea")
}

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the autenticated bot has access to.
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {

	// Ignore all messages created by the bot itself
	// This isn't required in this specific example but it's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}

	found := false
	for k, v := range bot_commands {
		if strings.Contains(m.Content, k) {
			s.ChannelMessageSend(m.ChannelID, v)
			found = true
			break
		}
	}
	if !found {
		if strings.Contains(m.Content, "!airhorn") {
			// Find the channel that the message came from.
			c, err := s.State.Channel(m.ChannelID)
			if err != nil {
				// Could not find channel.
				return
			}

			// Find the guild for that channel.
			g, err := s.State.Guild(c.GuildID)
			if err != nil {
				// Could not find guild.
				return
			}

			// Look for the message sender in that guild's current voice states.
			for _, vs := range g.VoiceStates {
				if vs.UserID == m.Author.ID {
					err = playSound(s, g.ID, vs.ChannelID)
					if err != nil {
						fmt.Println("Error playing sound:", err)
					}

					return
				}
			}
		} else if hamu_hunger_regexp.MatchString(m.Content) {
			s.ChannelMessageSend(m.ChannelID, "May I recommend a delicious Hamu Hamu?")
		} else if strings.HasPrefix(m.Content, "!add_command") {
			if _, exists := authorised_users[m.Author.ID]; exists {
				new_command := strings.Split(strings.Replace(m.Content, "!add_command ", "", 1), ":::")
				trigger, action := new_command[0], new_command[1]
				bot_commands[trigger] = action
				fmt.Println()
				fmt.Println(bot_commands)
				fmt.Println()
				bot_commands_json, _ := json.Marshal(bot_commands)
				write_err := ioutil.WriteFile("./botcommands.json", bot_commands_json, 0644)
				check(write_err)
			} else {
				s.ChannelMessageSend(m.ChannelID, "You are not authorised to use this command! Police!")
			}
		}
	}
}

// This function will be called (due to AddHandler above) every time a new
// guild is joined.
func guildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {

	if event.Guild.Unavailable {
		return
	}

	/*
		for _, channel := range event.Guild.Channels {
			if channel.ID == event.Guild.ID {
				_, _ = s.ChannelMessageSend(channel.ID, "I am a robot! Beep. Boop. Bop.")
				return
			}
		}
	*/
}

// loadSound attempts to load an encoded sound file from disk.
func loadSound() error {

	file, err := os.Open("airhorn.dca")
	if err != nil {
		fmt.Println("Error opening dca file :", err)
		return err
	}

	var opuslen int16

	for {
		// Read opus frame length from dca file.
		err = binary.Read(file, binary.LittleEndian, &opuslen)

		// If this is the end of the file, just return.
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			err := file.Close()
			if err != nil {
				return err
			}
			return nil
		}

		if err != nil {
			fmt.Println("Error reading from dca file :", err)
			return err
		}

		// Read encoded pcm from dca file.
		InBuf := make([]byte, opuslen)
		err = binary.Read(file, binary.LittleEndian, &InBuf)

		// Should not be any end of file errors
		if err != nil {
			fmt.Println("Error reading from dca file :", err)
			return err
		}

		// Append encoded pcm data to the buffer.
		buffer = append(buffer, InBuf)
	}
}

// playSound plays the current buffer to the provided channel.
func playSound(s *discordgo.Session, guildID, channelID string) (err error) {

	// Join the provided voice channel.
	vc, err := s.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		return err
	}

	// Sleep for a specified amount of time before playing the sound
	time.Sleep(250 * time.Millisecond)

	// Start speaking.
	vc.Speaking(true)

	// Send the buffer data.
	for _, buff := range buffer {
		vc.OpusSend <- buff
	}

	// Stop speaking
	vc.Speaking(false)

	// Sleep for a specificed amount of time before ending.
	time.Sleep(250 * time.Millisecond)

	// Disconnect from the provided voice channel.
	vc.Disconnect()

	return nil
}
