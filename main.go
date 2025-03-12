package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/schollz/progressbar/v3"
	"go.bug.st/serial"
)

const (
	ACK_RESPONSE       = 0x06
	FLASH_MODE_RESPONSE = 0xFF
	CMD_ERASE_FLASH     = 0x39
	CMD_READ_FLASH      = 0x52
	CMD_WRITE_FLASH     = 0x57
	WRITE_BLOCK_SIZE    = 0x400
	MEMORY_SIZE         = 0x3d800
)

func appendChecksum(data []byte) []byte {
	sum := 0
	for _, b := range data {
		sum += int(b)
	}
	return append(data, byte((sum+72)%256))
}

func main() {
	serialPort := flag.String("p", "/dev/ttyUSB0", "Serial port where the radio is connected")
	firmwareFile := flag.String("f", "", "File containing the firmware")
	flag.Parse()

	if *serialPort == "" || *firmwareFile == "" {
		flag.PrintDefaults()
		log.Fatal("Please provide both -port and -firmware arguments")
	}

	port, err := serial.Open(*serialPort, &serial.Mode{BaudRate: 115200})
	if err != nil {
		log.Fatal("Failed to open serial port:", err)
	}
	defer port.Close()

	fw, err := ioutil.ReadFile(*firmwareFile)
	if err != nil {
		log.Fatal("Failed to read firmware file:", err)
	}
	fmt.Printf("Firmware size: %d (0x%04X) bytes\n", len(fw), len(fw))

	port.Write(appendChecksum([]byte{CMD_READ_FLASH, 0, 0}))
	response := make([]byte, 4)
	port.Read(response)
	if response[0] != FLASH_MODE_RESPONSE {
		log.Fatal("Radio not in bootloader mode or not connected")
	}

	for _, part := range []byte{0x10, 0x55} {
		port.Write(appendChecksum([]byte{CMD_ERASE_FLASH, 0x33, 0x05, part}))
		ack := make([]byte, 1)
		port.Read(ack)
		if ack[0] != ACK_RESPONSE {
			log.Fatal("Failed to erase flash memory")
		}
	}

	fmt.Println("Flashing firmware...")
	fw = append(fw, bytes.Repeat([]byte{0x0}, MEMORY_SIZE-len(fw))...)

	bar := progressbar.NewOptions(MEMORY_SIZE,
		progressbar.OptionSetWidth(40),
		progressbar.OptionSetDescription("Writing"),
		progressbar.OptionShowBytes(true),
		progressbar.OptionOnCompletion(func() { fmt.Println("\nFlashing completed successfully!") }),
	)

	for offset := 0; offset < MEMORY_SIZE; offset += WRITE_BLOCK_SIZE {
		chunk := fw[offset : offset+WRITE_BLOCK_SIZE]
		payload := append([]byte{CMD_WRITE_FLASH, byte(offset >> 8), byte(offset & 0xFF)}, chunk...)
		port.Write(appendChecksum(payload))
		ack := make([]byte, 1)
		port.Read(ack)
		if ack[0] != ACK_RESPONSE {
			log.Fatal("Failed to write chunk at offset", offset)
		}

		bar.Add(len(chunk))
	}

	bar.Finish()
}
