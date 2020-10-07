// The MIT License
//
// Copyright (c) 2019 Apple, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package odoh

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"github.com/cisco/go-hpke"
	"log"
)

const (
	ODOH_VERSION       = uint16(0x0001)
	ODOH_SECRET_LENGTH = 32
	ODOH_PADDING_BYTE  = uint8(0)
	ODOH_LABEL_KEY_ID  = "odoh key id"
	ODOH_LABEL_KEY     = "odoh key"
	ODOH_LABEL_NONCE   = "odoh nonce"
	ODOH_LABEL_SECRET  = "odoh secret"
	ODOH_LABEL_QUERY   = "odoh query"
)

type ObliviousDoHConfigContents struct {
	KemID          hpke.KEMID
	KdfID          hpke.KDFID
	AeadID         hpke.AEADID
	PublicKeyBytes []byte
}

func CreateObliviousDoHConfigContents(kemID hpke.KEMID, kdfID hpke.KDFID, aeadID hpke.AEADID, publicKeyBytes []byte) (ObliviousDoHConfigContents, error) {
	suite, err := hpke.AssembleCipherSuite(kemID, kdfID, aeadID)
	if err != nil {
		return ObliviousDoHConfigContents{}, err
	}

	_, err = suite.KEM.Deserialize(publicKeyBytes)
	if err != nil {
		return ObliviousDoHConfigContents{}, err
	}

	return ObliviousDoHConfigContents{
		KemID:          kemID,
		KdfID:          kdfID,
		AeadID:         aeadID,
		PublicKeyBytes: publicKeyBytes,
	}, nil
}

type ObliviousDoHConfig struct {
	Version  uint16
	Contents ObliviousDoHConfigContents
}

func CreateObliviousDoHConfig(contents ObliviousDoHConfigContents) ObliviousDoHConfig {
	return ObliviousDoHConfig{
		Version:  ODOH_VERSION,
		Contents: contents,
	}
}

func (c ObliviousDoHConfig) Marshal() []byte {
	marshalledConfig := c.Contents.Marshal()

	buffer := make([]byte, 4)
	binary.BigEndian.PutUint16(buffer[0:], uint16(c.Version))
	binary.BigEndian.PutUint16(buffer[2:], uint16(len(marshalledConfig)))

	configBytes := append(buffer, marshalledConfig...)
	return configBytes
}

func UnmarshalObliviousDoHConfig(buffer []byte) (ObliviousDoHConfig, error) {
	version := binary.BigEndian.Uint16(buffer[0:])
	length := binary.BigEndian.Uint16(buffer[2:])
	if len(buffer[4:]) < int(length) {
		return ObliviousDoHConfig{}, errors.New("Invalid serialized ObliviousDoHConfig")
	}

	configContents := UnmarshalObliviousDoHConfigContents(buffer[4:])

	return ObliviousDoHConfig{
		Version:  version,
		Contents: configContents,
	}, nil
}

func (k ObliviousDoHConfigContents) KeyID() []byte {
	suite, err := hpke.AssembleCipherSuite(k.KemID, k.KdfID, k.AeadID)
	if err != nil {
		return nil
	}

	identifiers := make([]byte, 8)
	binary.BigEndian.PutUint16(identifiers[0:], uint16(k.KemID))
	binary.BigEndian.PutUint16(identifiers[2:], uint16(k.KdfID))
	binary.BigEndian.PutUint16(identifiers[4:], uint16(k.AeadID))
	binary.BigEndian.PutUint16(identifiers[6:], uint16(len(k.PublicKeyBytes)))
	config := append(identifiers, k.PublicKeyBytes...)

	prk := suite.KDF.Extract(nil, config)
	identifier := suite.KDF.Expand(prk, []byte(ODOH_LABEL_KEY_ID), suite.KDF.OutputSize())

	return identifier
}

func (k ObliviousDoHConfigContents) Marshal() []byte {
	identifiers := make([]byte, 8)
	binary.BigEndian.PutUint16(identifiers[0:], uint16(k.KemID))
	binary.BigEndian.PutUint16(identifiers[2:], uint16(k.KdfID))
	binary.BigEndian.PutUint16(identifiers[4:], uint16(k.AeadID))
	binary.BigEndian.PutUint16(identifiers[6:], uint16(len(k.PublicKeyBytes)))

	response := append(identifiers, k.PublicKeyBytes...)
	return response
}

func UnmarshalObliviousDoHConfigContents(buffer []byte) ObliviousDoHConfigContents {
	kemId := binary.BigEndian.Uint16(buffer[0:])
	kdfId := binary.BigEndian.Uint16(buffer[2:])
	AeadId := binary.BigEndian.Uint16(buffer[4:])
	pkLen := binary.BigEndian.Uint16(buffer[6:])

	pkBytes := buffer[8 : 8+pkLen]

	var KemID hpke.KEMID
	var KdfID hpke.KDFID
	var AeadID hpke.AEADID

	switch kemId {
	case 0x0010:
		KemID = hpke.DHKEM_P256
		break
	case 0x0012:
		KemID = hpke.DHKEM_P521
		break
	case 0x0020:
		KemID = hpke.DHKEM_X25519
		break
	case 0x0021:
		KemID = hpke.DHKEM_X448
		break
	case 0xFFFE:
		KemID = hpke.KEM_SIKE503
		break
	case 0xFFFF:
		KemID = hpke.KEM_SIKE751
		break
	default:
		log.Fatalln("Unable to find the correct KEM ID Type")
	}

	switch kdfId {
	case 0x0001:
		KdfID = hpke.KDF_HKDF_SHA256
		break
	case 0x0002:
		KdfID = hpke.KDF_HKDF_SHA384
		break
	case 0x0003:
		KdfID = hpke.KDF_HKDF_SHA512
		break
	default:
		log.Fatalln("Unable to find correct KDF ID Type")
	}

	switch AeadId {
	case 0x0001:
		AeadID = hpke.AEAD_AESGCM128
		break
	case 0x0002:
		AeadID = hpke.AEAD_AESGCM256
		break
	case 0x0003:
		AeadID = hpke.AEAD_CHACHA20POLY1305
		break
	default:
		log.Fatalln("Unable to find correct AEAD ID Type")
	}

	return ObliviousDoHConfigContents{
		KemID:          KemID,
		KdfID:          KdfID,
		AeadID:         AeadID,
		PublicKeyBytes: pkBytes,
	}
}

func (k ObliviousDoHConfigContents) PublicKey() []byte {
	return k.PublicKeyBytes
}

func (k ObliviousDoHConfigContents) CipherSuite() (hpke.CipherSuite, error) {
	return hpke.AssembleCipherSuite(k.KemID, k.KdfID, k.AeadID)
}

type ObliviousDNSKeyPair struct {
	Config    ObliviousDoHConfig
	SecretKey hpke.KEMPrivateKey
	Seed      []byte
}

func (k ObliviousDNSKeyPair) CipherSuite() (hpke.CipherSuite, error) {
	return hpke.AssembleCipherSuite(k.Config.Contents.KemID, k.Config.Contents.KdfID, k.Config.Contents.AeadID)
}

func DeriveFixedKeyPairFromSeed(kemID hpke.KEMID, kdfID hpke.KDFID, aeadID hpke.AEADID, ikm []byte) (ObliviousDNSKeyPair, error) {
	suite, err := hpke.AssembleCipherSuite(kemID, kdfID, aeadID)
	if err != nil {
		return ObliviousDNSKeyPair{}, err
	}

	sk, pk, err := suite.KEM.DeriveKeyPair(ikm)
	if err != nil {
		return ObliviousDNSKeyPair{}, err
	}

	config := ObliviousDoHConfig{
		Contents: ObliviousDoHConfigContents{
			KemID:          kemID,
			KdfID:          kdfID,
			AeadID:         aeadID,
			PublicKeyBytes: suite.KEM.Serialize(pk),
		},
	}

	return ObliviousDNSKeyPair{config, sk, ikm}, nil
}

func CreateKeyPair(kemID hpke.KEMID, kdfID hpke.KDFID, aeadID hpke.AEADID) (ObliviousDNSKeyPair, error) {
	suite, err := hpke.AssembleCipherSuite(kemID, kdfID, aeadID)
	if err != nil {
		return ObliviousDNSKeyPair{}, err
	}

	ikm := make([]byte, suite.KEM.PrivateKeySize())
	rand.Reader.Read(ikm)
	sk, pk, err := suite.KEM.DeriveKeyPair(ikm)
	if err != nil {
		return ObliviousDNSKeyPair{}, err
	}

	config := ObliviousDoHConfig{
		Contents: ObliviousDoHConfigContents{
			KemID:          kemID,
			KdfID:          kdfID,
			AeadID:         aeadID,
			PublicKeyBytes: suite.KEM.Serialize(pk),
		},
	}

	return ObliviousDNSKeyPair{config, sk, ikm}, nil
}

type QueryContext struct {
	odohSecret []byte
	suite      hpke.CipherSuite
	query      []byte
	publicKey  ObliviousDoHConfigContents
}

func (c QueryContext) DecryptResponse(message ObliviousDNSMessage) ([]byte, error) {
	aad := append([]byte{byte(ResponseType)}, []byte{0x00, 0x00}...) // 0-length encoded KeyID

	odohPRK := c.suite.KDF.Extract(c.query, c.odohSecret)
	key := c.suite.KDF.Expand(odohPRK, []byte(ODOH_LABEL_KEY), c.suite.AEAD.KeySize())
	nonce := c.suite.KDF.Expand(odohPRK, []byte(ODOH_LABEL_NONCE), c.suite.AEAD.NonceSize())

	aead, err := c.suite.AEAD.New(key)
	if err != nil {
		return nil, err
	}

	return aead.Open(nil, nonce, message.EncryptedMessage, aad)
}

type ResponseContext struct {
	query      []byte
	suite      hpke.CipherSuite
	odohSecret []byte
}

func (c ResponseContext) EncryptResponse(response *ObliviousDNSResponse) (ObliviousDNSMessage, error) {
	aad := append([]byte{byte(ResponseType)}, []byte{0x00, 0x00}...) // 0-length encoded KeyID

	odohPRK := c.suite.KDF.Extract(c.query, c.odohSecret)
	key := c.suite.KDF.Expand(odohPRK, []byte(ODOH_LABEL_KEY), c.suite.AEAD.KeySize())
	nonce := c.suite.KDF.Expand(odohPRK, []byte(ODOH_LABEL_NONCE), c.suite.AEAD.NonceSize())

	aead, err := c.suite.AEAD.New(key)
	if err != nil {
		return ObliviousDNSMessage{}, err
	}

	ciphertext := aead.Seal(nil, nonce, response.Marshal(), aad)

	odohMessage := ObliviousDNSMessage{
		KeyID:            nil,
		MessageType:      ResponseType,
		EncryptedMessage: ciphertext,
	}

	return odohMessage, nil
}

func (targetKey ObliviousDoHConfigContents) EncryptQuery(query *ObliviousDNSQuery) (ObliviousDNSMessage, QueryContext, error) {
	suite, err := hpke.AssembleCipherSuite(targetKey.KemID, targetKey.KdfID, targetKey.AeadID)
	if err != nil {
		return ObliviousDNSMessage{}, QueryContext{}, err
	}

	pkR, err := suite.KEM.Deserialize(targetKey.PublicKeyBytes)
	if err != nil {
		return ObliviousDNSMessage{}, QueryContext{}, err
	}

	enc, ctxI, err := hpke.SetupBaseS(suite, rand.Reader, pkR, []byte(ODOH_LABEL_QUERY))
	if err != nil {
		return ObliviousDNSMessage{}, QueryContext{}, err
	}

	keyID := targetKey.KeyID()
	keyIDLength := make([]byte, 2)
	binary.BigEndian.PutUint16(keyIDLength, uint16(len(keyID)))
	aad := append([]byte{byte(QueryType)}, keyIDLength...)
	aad = append(aad, keyID...)

	encodedMessage := query.Marshal()
	ct := ctxI.Seal(aad, encodedMessage)
	odohSecret := ctxI.Export([]byte(ODOH_LABEL_SECRET), ODOH_SECRET_LENGTH)

	return ObliviousDNSMessage{
			KeyID:            targetKey.KeyID(),
			MessageType:      QueryType,
			EncryptedMessage: append(enc, ct...),
		}, QueryContext{
			odohSecret: odohSecret,
			suite:      suite,
			query:      query.Marshal(),
			publicKey:  targetKey,
		}, nil
}

func validateMessagePadding(padding []byte) bool {
	validPadding := 1
	for _, v := range padding {
		validPadding &= subtle.ConstantTimeByteEq(v, ODOH_PADDING_BYTE)
	}
	return validPadding == 1
}

func (privateKey ObliviousDNSKeyPair) DecryptQuery(message ObliviousDNSMessage) (*ObliviousDNSQuery, ResponseContext, error) {
	suite, err := hpke.AssembleCipherSuite(privateKey.Config.Contents.KemID, privateKey.Config.Contents.KdfID, privateKey.Config.Contents.AeadID)
	if err != nil {
		return nil, ResponseContext{}, err
	}

	keySize := suite.KEM.PublicKeySize()
	enc := message.EncryptedMessage[0:keySize]
	ct := message.EncryptedMessage[keySize:]

	ctxR, err := hpke.SetupBaseR(suite, privateKey.SecretKey, enc, []byte(ODOH_LABEL_QUERY))
	if err != nil {
		return nil, ResponseContext{}, err
	}

	odohSecret := ctxR.Export([]byte(ODOH_LABEL_SECRET), ODOH_SECRET_LENGTH)

	keyID := privateKey.Config.Contents.KeyID()
	keyIDLength := make([]byte, 2)
	binary.BigEndian.PutUint16(keyIDLength, uint16(len(keyID)))
	aad := append([]byte{byte(QueryType)}, keyIDLength...)
	aad = append(aad, keyID...)

	dnsMessage, err := ctxR.Open(aad, ct)
	if err != nil {
		return nil, ResponseContext{}, err
	}

	query, err := UnmarshalQueryBody(dnsMessage)
	if err != nil {
		return nil, ResponseContext{}, err
	}

	if !validateMessagePadding(query.Padding) {
		return nil, ResponseContext{}, errors.New("invalid padding")
	}

	responseContext := ResponseContext{
		odohSecret: odohSecret,
		suite:      suite,
		query:      query.Marshal(),
	}

	return query, responseContext, nil
}

func SealQuery(dnsQuery []byte, publicKey ObliviousDoHConfigContents) (ObliviousDNSMessage, QueryContext, error) {
	odohQuery := CreateObliviousDNSQuery(dnsQuery, 0)

	odohMessage, queryContext, err := publicKey.EncryptQuery(odohQuery)
	if err != nil {
		return ObliviousDNSMessage{}, QueryContext{}, err
	}

	return odohMessage, queryContext, nil
}

func (c QueryContext) OpenAnswer(odohResponse ObliviousDNSMessage) ([]byte, error) {
	if odohResponse.MessageType != ResponseType {
		return nil, errors.New("answer is not a valid response type")
	}

	decryptedResponseBytes, err := c.DecryptResponse(odohResponse)
	if err != nil {
		return nil, errors.New("unable to decrypt the obtained response using the symmetric key sent")
	}

	decryptedResponse, err := UnmarshalResponseBody(decryptedResponseBytes)
	if err != nil {
		return nil, err
	}

	return decryptedResponse.DnsMessage, nil
}
