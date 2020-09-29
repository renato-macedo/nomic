package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	texttospeech "cloud.google.com/go/texttospeech/apiv1"
	"github.com/jonas747/dca"
	texttospeechpb "google.golang.org/genproto/googleapis/cloud/texttospeech/v1"
)

var voiceChannel *discordgo.VoiceConnection
var buffer = make([][]byte, 0)

func main() {
	fmt.Println("Hello")
	token := os.Getenv("TOKEN")
	fmt.Println(token)

	discord, err := discordgo.New("Bot " + token)

	if err != nil {
		log.Fatal(err)
	}

	// Open a websocket connection to Discord and begin listening.
	err = discord.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	discord.AddHandler(requestToJoin)

	discord.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildVoiceStates)

	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("Bot is now running.  Press CTRL-C to exit.")

	// Instantiates a client.
	ctx := context.Background()
	client, err := texttospeech.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}

	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		text := scanner.Text()

		sound, err := textToSpeech(ctx, text, client)

		if err != nil {
			log.Fatal(err)
		}

		buff := bytes.NewBuffer(sound)
		err = dcaEncode(buff)
		if err != nil {
			log.Fatal(err)
		}

		err = loadSound()
		if err != nil {
			log.Fatal(err)
		}

		filename := "output.mp3"
		err = ioutil.WriteFile(filename, sound, 0644)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Audio content written to file: %v\n", filename)

		playSound(voiceChannel)
	}

	discord.Close()

}

func textToSpeech(ctx context.Context, text string, client *texttospeech.Client) ([]byte, error) {

	fmt.Println("calling tts api")
	// Perform the text-to-speech request on the text input with the selected
	// voice parameters and audio file type.
	req := texttospeechpb.SynthesizeSpeechRequest{
		// Set the text input to be synthesized.
		Input: &texttospeechpb.SynthesisInput{
			InputSource: &texttospeechpb.SynthesisInput_Text{Text: text},
		},
		// Build the voice request, select the language code ("en-US") and the SSML
		// voice gender ("neutral").
		Voice: &texttospeechpb.VoiceSelectionParams{
			LanguageCode: "pt-BR",
			SsmlGender:   texttospeechpb.SsmlVoiceGender_NEUTRAL,
		},
		// Select the type of audio file you want returned.
		AudioConfig: &texttospeechpb.AudioConfig{
			AudioEncoding: texttospeechpb.AudioEncoding_MP3,
		},
	}

	resp, err := client.SynthesizeSpeech(ctx, &req)
	if err != nil {
		return nil, err
	}

	return resp.AudioContent, nil
}

// playSound plays the current buffer to the provided channel.
func playSound(vc *discordgo.VoiceConnection) {
	if vc == nil {
		return
	}
	fmt.Println("Playing sound")

	// Sleep for a specified amount of time before playing the sound
	time.Sleep(250 * time.Millisecond)

	// Start speaking.
	vc.Speaking(true)

	for _, buff := range buffer {
		vc.OpusSend <- buff
	}

	// Stop speaking
	vc.Speaking(false)

	// Sleep for a specificed amount of time before ending.
	time.Sleep(250 * time.Millisecond)
}

func requestToJoin(s *discordgo.Session, m *discordgo.MessageCreate) {

	if m.Author.ID == s.State.User.ID {
		return
	}

	fmt.Println(m.Content)
	// check if the message is "!airhorn"
	if strings.HasPrefix(m.Content, "!join") {
		s.ChannelMessageSend(m.ChannelID, "Pong!")

		// Find the channel that the message came from.
		c, err := s.State.Channel(m.ChannelID)
		if err != nil {
			log.Println("Could not find channel.", err)
			// Could not find channel.
			return
		}

		// Find the guild for that channel.
		g, err := s.State.Guild(c.GuildID)
		if err != nil {
			log.Println("Could not find guild", err)
			// Could not find guild.
			return
		}

		// Look for the message sender in that guild's current voice states.
		for _, vs := range g.VoiceStates {
			fmt.Println("userID", vs.UserID, "authorID", m.Author.ID)

			if vs.UserID == m.Author.ID {
				voiceChannel, err = s.ChannelVoiceJoin(g.ID, vs.ChannelID, false, true)
				if err != nil {
					log.Println("kkkk", err)
				}
			}
		}
	}
}

func dcaEncode(data io.Reader) error {
	// Encoding a file and saving it to disk

	encodeSession, err := dca.EncodeMem(data, dca.StdEncodeOptions)
	if err != nil {
		return err
	}
	// Make sure everything is cleaned up, that for example the encoding process if any issues happened isnt lingering around
	defer encodeSession.Cleanup()

	output, err := os.Create("output.dca")
	if err != nil {
		return err
	}

	io.Copy(output, encodeSession)
	return nil
}

// loadSound attempts to load an encoded sound file from disk.
func loadSound() error {

	file, err := os.Open("output.dca")
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
