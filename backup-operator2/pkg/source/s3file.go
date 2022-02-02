package source

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"io"
)

type S3File struct {
	BucketName       string
	Path             string
	S3Client         *s3.S3
	obj              *s3.ListObjectVersionsOutput
	objLatestVersion *s3.ObjectVersion
	objByte          []byte
	objByteReadIndex int64
	ctx              context.Context
	downloader       *s3manager.Downloader
}

func NewS3File(ctx context.Context, bucketname, path string, s3client *s3.S3) (*S3File, error) {
	s3file := &S3File{
		BucketName: bucketname,
		Path:       path,
		S3Client:   s3client,
		ctx:        ctx,
		downloader: s3manager.NewDownloaderWithClient(s3client),
	}
	_, err := s3file.GetObject()
	if err != nil {
		return s3file, err
	}

	return s3file, nil
}

func (s *S3File) GetObjectByte() []byte {
	return s.objByte
}

func (s *S3File) GetObject() (*s3.ListObjectVersionsOutput, error) {
	if s.obj != nil {
		return s.obj, nil
	}

	obj, err := s.S3Client.ListObjectVersionsWithContext(s.ctx, &s3.ListObjectVersionsInput{
		Bucket: aws.String(s.BucketName),
		Prefix: aws.String(s.Path),
	})
	for _, version := range obj.Versions {
		if *version.IsLatest {
			s.objLatestVersion = version
		}
	}

	if err != nil {
		return nil, fmt.Errorf("could not get object '%s': %v", s.URL(), err)
	}
	s.obj = obj

	return s.obj, nil
}

func (s *S3File) Read(b []byte) (n int, err error) {
	if s.objByteReadIndex >= *s.objLatestVersion.Size {
		s.objByteReadIndex = 0
		return 0, io.EOF
	}
	in := aws.NewWriteAtBuffer(b)
	bytesRead, err := s.downloader.DownloadWithContext(s.ctx, in, &s3.GetObjectInput{
		Bucket: aws.String(s.BucketName),
		Key:    aws.String(s.Path),
		Range:  aws.String(fmt.Sprintf("bytes=%d-%d", s.objByteReadIndex, int(s.objByteReadIndex)+len(b)-1)),
	})
	s.objByteReadIndex += bytesRead
	if err != nil {
		s.objByteReadIndex = 0
		return 0, fmt.Errorf("could not download file '%s': %v", s.URL(), err)
	}
	return int(bytesRead), nil
}

func (s *S3File) RemoveDeleteMarker() error {
	if len(s.obj.DeleteMarkers) != 0 {
		//Remove delete Markers
		_, err := s.S3Client.DeleteObjectWithContext(s.ctx, &s3.DeleteObjectInput{
			Bucket:    aws.String(s.BucketName),
			Key:       s.obj.DeleteMarkers[0].Key,
			VersionId: s.obj.DeleteMarkers[0].VersionId,
		})
		if err != nil {
			return fmt.Errorf("could not remove 'delete marker' on s3file '%s': %v", s.URL(), err)
		}
	}
	return nil
}

func (s *S3File) RestoreFromGlacier() error {
	if !s.IsGlacier() {
		return fmt.Errorf("s3file '%s' not in glacier", s.URL())
	}
	err := s.RemoveDeleteMarker()
	if err != nil {
		return err
	}
	_, err = s.S3Client.RestoreObjectWithContext(s.ctx, &s3.RestoreObjectInput{
		Bucket:    aws.String(s.BucketName),
		Key:       s.obj.Versions[0].Key,
		VersionId: s.obj.Versions[0].VersionId,
		RestoreRequest: &s3.RestoreRequest{
			Days: aws.Int64(1),
			GlacierJobParameters: &s3.GlacierJobParameters{
				Tier: aws.String(s3.TierExpedited),
			},
			Description:      nil,
			OutputLocation:   nil,
			SelectParameters: nil,
			Tier:             nil,
			Type:             nil,
		},
	})
	if err != nil {
		return fmt.Errorf("could not restore s3file '%s' from glacier: %v", s.URL(), err)
	}
	return nil
}

func (s *S3File) GetGlacierStatus() (*s3.HeadObjectOutput, error) {
	res, err := s.S3Client.HeadObjectWithContext(s.ctx, &s3.HeadObjectInput{
		Bucket:    aws.String(s.BucketName),
		Key:       aws.String(s.String()),
		VersionId: s.obj.Versions[0].VersionId,
	})

	return res, err
}

func (s *S3File) IsGlacier() bool {
	return *s.obj.Versions[0].StorageClass == s3.StorageClassGlacier
}

func (s *S3File) String() string {
	return s.BucketName + "/" + s.Path
}

func (s *S3File) URL() string {
	return "s3://" + s.String()
}
