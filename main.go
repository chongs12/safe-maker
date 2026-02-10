package main

import (
	safeflow "github.com/safeflow-project/safeflow/kitex_gen/safeflow/imagemoderationservice"
	"log"
)

func main() {
	svr := safeflow.NewServer(new(ImageModerationServiceImpl))

	err := svr.Run()

	if err != nil {
		log.Println(err.Error())
	}
}
