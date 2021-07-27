# in-memory-key-value-db-go

in-memory-key-value-db-go is an in-memory key-value store with small test server that is suitable for applications running on a single machine. it`s written in GOlang. Being essentially a thread-safe map[string]interface{} with expiration times (TTL), it doesn't need to serialize or transmit contents over the network. Might be easy configured as you need. 

## How to use?

Import this in your project

	import (
		"github.com/Doingthingsjke/memory-db-go/server"
	)

Then u can init your new in-memory db on server for example

	srv := &Server{
		db: *newDB(5 * time.Minute, 10 * time.Minute),
	}

or u can init with db.json (your local file)

	db: *newDbFrom(5 * time.Minute, 10 * time.Minute),

Use it in console like this:

	set a 1 3000
	get a
	add b 1 8080
	delete b
	exit

You can control some connections on you server, saving in-memory-values into db.json local after closing server.
