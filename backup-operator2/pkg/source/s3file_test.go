package source

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"io"
	"log"
	"os"
	"testing"
)

// To test this function, you have to have proper crm-admin credentials
// aws-vault exec crm-admin
// go test
func TestS3FileDownloadStream(t *testing.T) {
	sess, err := session.NewSession()
	if err != nil {
		log.Fatalf("could not initialize aws session: %v", err)
	}
	s3file, err := NewS3File(context.TODO(), "db-backup-kt.accp.kronos-crm.com", "2021/12/01/abex__109.sql.xz", s3.New(sess))
	if err != nil {
		log.Fatalf("could not initialize s3file: %v", err)
	}
	f, _ := os.CreateTemp("/tmp", "test")
	defer f.Close()
	defer os.Remove(f.Name())
	_, err = io.Copy(f, s3file)
	if err != nil {
		fmt.Printf("error: %v", err)
	}
	finfo, err := os.Stat(f.Name())
	if err != nil {
		log.Fatalf("%v", err)
	}
	if finfo.Size() != *s3file.objLatestVersion.Size {
		log.Fatalf("size does not match '%d' != '%d'", finfo.Size(), *s3file.objLatestVersion.Size)
	}
}
