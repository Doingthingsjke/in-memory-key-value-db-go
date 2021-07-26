package server

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)


type Server struct {
	listener net.Listener
	quit     chan struct{}
	exited   chan struct{}
	db       MemoryDB
	connections  map[int]net.Conn
	connCloseTimeout time.Duration
}

func NewServer() *Server {
	l, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatal("Failed to create listener", err.Error())
	}
	// Инициализируем in-memory db с временем жизни равным 3 минутам и удалением кеша каждые 10 минут и с сохраненным бэкапом
	srv := &Server{
		listener: l,
		quit: make(chan struct{}),
		exited: make(chan struct{}),
		db: *newDB(5 * time.Minute, 10 * time.Minute),
		connections: map[int]net.Conn{},
		connCloseTimeout: 10 * time.Second,
	}
	go srv.serve()
	return srv
}

func (srv *Server) serve() {
	var id int
	fmt.Println("Listening for clients")
	for {
		select {
		case <-srv.quit:
			fmt.Println("Droping down the server")
			err := srv.listener.Close()
			if err != nil {
				fmt.Println("Couldn`t close listener", err.Error())
			}
			if len(srv.connections) > 0 {
				srv.warnConnections(srv.connCloseTimeout)
				<-time.After(srv.connCloseTimeout)
				srv.closeConnections()
			}
			close(srv.exited)
			return 
		default:
			tcpListener := srv.listener.(*net.TCPListener)
			err := tcpListener.SetDeadline(time.Now().Add(2 * time.Second))
			if err != nil {
				fmt.Println("Failed to set listener deadline", err.Error())
			}

			conn, err := tcpListener.Accept()
			if oppErr, ok := err.(*net.OpError); ok && oppErr.Timeout() {
				continue
			}
			if err != nil {
				fmt.Println("Failed to accept connection", err.Error())
			}

			if len(srv.connections) >= maxUsers {
				write(conn, "The MemoryDB server is full, try to connect later")
				conn.Close()
			} else {
				srv.connections[id] = conn
				write(conn, "Welcome to MemoryDB server")
			}
			
			go func (connID int)  {
				if len(srv.connections) >= maxUsers {
					fmt.Println("Client with id", connID, "was trying to connect, but server is full")
					// if err := conn.Close(); err != nil {
					// 	fmt.Println("Couldn`t close connection", err.Error())
					// }
				} else {
					fmt.Println("Client with id", connID, "joined the server")
					srv.handleConn(conn)
					delete(srv.connections, connID)
					fmt.Println("Client with id", connID, "left the server")
				}
			}(id)
			id++
		}
	}
}

func write(conn net.Conn, s string) {
	_, err := fmt.Fprintf(conn, "%s\n-> ", s)
	if err != nil {
		log.Fatal(err)
	}
}

func (srv *Server) handleConn(conn net.Conn) {
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		l := strings.ToLower(strings.TrimSpace(scanner.Text()))
		values := strings.Split(l, " ")
		switch {
		case len(values) == 4 && values[0] == "set":
			time,_ := time.ParseDuration(values[3])
			if len(srv.db.items) >= maxSize {
				fmt.Println("Database is full of size!")
				write(conn, "You need to delete something from db before setting new, it have full of size")
			} else {
				srv.db.SET(values[1], values[2], time)
				write(conn, fmt.Sprintf("Seted value to %s. it will be removed in %d", values[1], time))
			}
		case len(values) == 4 && values[0] == "add":
			time,_ := time.ParseDuration(values[3])
			fmt.Println(values[0], values[1], values[2], values[3], time)
			
			if len(srv.db.items) >= maxSize {
				fmt.Println("Database is full of size!")
				write(conn, "You need to delete something from db before adding new, it have full of size")
			} else {
				err := srv.db.ADD(values[1], values[2], time)
				if err == nil {
					write(conn, fmt.Sprintf("Added new value %s. it will be removed in %d", values[1], time))
				} else {
					write(conn, err.Error())
				}
			}
		case len(values) == 2 && values[0] == "get":
			val, found := srv.db.GET(values[1])
			if !found {
				write(conn, fmt.Sprintf("Key %s wasn`t found, actualy deleted", values[1]))
			} else {
				str := fmt.Sprintf("%v", val)
				write(conn, fmt.Sprintf("Desired value: %s", str))
			}
		case len(values) == 2 && values[0] == "delete":
			if srv.db.DELETE(values[1]) {
				write(conn, fmt.Sprintf("Deleted value %s", values[1]))
			} else {
				write(conn, fmt.Sprintf("Key %s wasn`t found", values[1]))
			}
		case len(values) == 1 && values[0] == "exit":
			if err := conn.Close(); err != nil {
				fmt.Println("Couldn`t close connection", err.Error())
			}
		default:
			write(conn, fmt.Sprintf("Unknown: %s", l))
		}
	}
}

func (srv *Server) warnConnections(timeout time.Duration) {
	for _, conn := range srv.connections {
		write(conn, fmt.Sprintf("Host wants to drop down the server in: %s", timeout.String()))
	}
}

func (srv *Server) closeConnections() {
	fmt.Println("Closing all connections")
	for id, conn := range srv.connections {
		err := conn.Close()
		if err != nil {
			fmt.Println("Couldn`t close connection with id:", id)
		}
	}
}

func (srv *Server) Stop() {
	fmt.Println("Stopping the MemoryDB server")
	close(srv.quit)
	<-srv.exited
	fmt.Println("Saving in-memory db records...")
	srv.db.saveToFile()
	fmt.Println("MemoryDB server is stopped")
}