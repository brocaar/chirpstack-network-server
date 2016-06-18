/*

Package lorawan provides structures and tools to read and write LoraWAN
messages from and to a slice of bytes.

The following structures are implemented (+ fields):

    * PHYPayload    (MHDR | MACPayload | MIC)
    * MACPayload    (FHDR | FPort | FRMPayload)
    * FHDR          (DevAddr | FCtrl | FCnt | FOpts)

The Following message types (MType) are implemented:

    * JoinRequest
    * JoinAccept
    * UnconfirmedDataUp
    * UnconfirmedDataDown
    * ConfirmedDataUp
    * ConfirmedDataDown
    * Proprietary

The following MAC commands (and their optional payloads) are implemented:

    * LinkCheckReq
    * LinkCheckAns
    * LinkADRReq
    * LinkADRAns
    * DutyCycleReq
    * DutyCycleAns
    * RXParamSetupReq
    * RXParamSetupAns
    * DevStatusReq
    * DevStatusAns
    * NewChannelReq
    * NewChannelAns
    * RXTimingSetupReq
    * RXTimingSetupAns
    * Proprietary commands (0x80 - 0xFF) can be registered with RegisterProprietaryMACCommand

Support for calculating and setting the MIC is done by calling SetMIC():

    err := phyPayload.SetMIC(key)

Validating the MIC is done by calling ValidateMIC():

    valid, err := phyPayload.ValidateMIC(key)

Encryption and decryption of the MACPayload (for join-accept) is done by
calling EncryptJoinAcceptPayload() and DecryptJoinAcceptPayload(). Note that you need to
call SetMIC BEFORE encryption.

    err := phyPayload.EncryptJoinAcceptPayload(key)
    err := phyPayload.DecryptJoinAcceptPayload(key)

Encryption and decryption of the FRMPayload is done by calling
EncryptFRMPayload() and DecryptFRMPayload(). After encryption (and thus
before decryption), the bytes are stored in the DataPayload struct.

    err := phyPayload.EncryptFRMPayload(key)
    err := phyPayload.DecryptFRMPayload(key)

All payloads implement the Payload interface. Based on the MIC value, you
should be able to know to which type to cast the Payload value, so you will
be able to access its fields.

See the examples section of the documentation for concrete usage examples
of this package.

When using this package, knowledge about the LoRaWAN specification is needed.
You can download the LoRaWAN specification here:
https://www.lora-alliance.org/For-Developers/LoRaWANDevelopers

*/
package lorawan
