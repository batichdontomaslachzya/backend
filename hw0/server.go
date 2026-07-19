package main

import (
	"fmt"
	"net"
)

func handleConnection(conn net.Conn) {
	defer conn.Close()

	_, err := conn.Write([]byte("OK\n"))

	if err != nil {
		fmt.Println("Ошибка отправки ответа:", err)
		return
	}

	fmt.Println("Ответ отправлен")
}

func main() {
	listener, err := net.Listen("tcp", ":8080")

	if err != nil {
		fmt.Println("Ошибка запуска сервера:", err)
		return
	}

	defer listener.Close()

	fmt.Println("Сервер запущен на порту 8080")

	for {
		conn, err := listener.Accept()

		if err != nil {
			fmt.Println("Ошибка подключения:", err)
			continue
		}

		go handleConnection(conn)
	}
}
