package main

import (
	"bufio"
	"io"

	"code.google.com/p/goprotobuf/proto"
	dto "github.com/prometheus/client_model/go"
)

type protobufEncoder interface {
	Encode(proto.Message) error
}

// decoder reads Samples from an input stream using a dto.Decoder.
type decoder struct {
	dec *dto.Decoder
	pb  *dto.Sample
}

func newDecoder(r io.Reader) *decoder {
	return &decoder{
		dec: dto.NewDecoder(r),
		pb:  new(dto.Sample),
	}
}

func (dec *decoder) Decode(s *Sample) error {
	dec.pb.Reset()

	if err := dec.dec.Decode(dec.pb); err != nil {
		return err
	}

	var (
		timestamp = dec.pb.GetTime()
		labels    = make([]Label, 0, len(dec.pb.GetLabel()))
	)

	for _, label := range dec.pb.GetLabel() {
		labels = append(labels, Label{label.GetKey(), label.GetVal()})
	}

	s.Name = dec.pb.GetName()
	s.Value = dec.pb.GetValue()
	s.Labels = labels

	if timestamp != 0 {
		s.Timestamp = timestamp
	}

	return nil
}

// Encoder writes Samples to a protobufEncoder.
type encoder struct {
	enc protobufEncoder
	pb  *dto.Sample
}

func newEncoder(enc protobufEncoder) *encoder {
	return &encoder{
		enc: enc,
		pb:  new(dto.Sample),
	}
}

func (enc *encoder) Encode(s *Sample) error {
	var pb = enc.pb

	pb.Name = &s.Name
	pb.Value = &s.Value
	pb.Time = &s.Timestamp
	pb.Label = pb.Label[:0]

	for _, label := range s.Labels {
		pb.Label = append(pb.Label, &dto.Label{
			Key: proto.String(label.Key),
			Val: proto.String(label.Val),
		})
	}

	return enc.enc.Encode(pb)
}

// textEncoder writes newline delimited text-encoded protobuf messages to an
// output stream.
type textEncoder struct {
	w *bufio.Writer
}

func newTextEncoder(w io.Writer) *textEncoder {
	return &textEncoder{bufio.NewWriter(w)}
}

func (enc *textEncoder) Encode(pb proto.Message) error {
	if err := proto.CompactText(enc.w, pb); err != nil {
		return err
	}

	if err := enc.w.WriteByte('\n'); err != nil {
		return err
	}

	return enc.w.Flush()
}
