# Activating nodes

After setting up an application and node in LoRa Server (you can do this
throught the JSON-RPC API or the web-interface), you need to setup your node.
This document will describe this process for different types of nodes.

The following example data is used:

```
DevEUI: 0102030405060708
AppEUI: 0807060504030201
AppKey: 01020304050607080910111213141516
```

## RN2483 / RN2903

Through a serial terminal, use the following commands:

```
mac set deveui 0102030405060708
mac set appeui 0807060504030201
mac set appkey 01020304050607080910111213141516
mac join otaa
```

## iM880A-L / iM880B-L

Make sure your node is running a recent firmware! These steps were tested
with a node running v1.14.

Use WiMOD LoRaWAN EndNode Studio for the following actions:

To set the ``DevEUI``:

1. Go to *Extras* -> *Factory Settings*
2. Click *Set Customer Mode*
3. Enter the *Device EUI* as ``0807060504030201``. **Note**: this field
   must be entered as LSBF (meaning it is the reverse value as created in
   LoRa Server)!
4. Click *Set Device EUI*
5. Click *Set Application Mode*

To set the ``AppEUI`` and ``AppKey``:

1. Go back to *LoRaWAN Node*
2. Enter the *Application EUI* as ``0102030405060708``. **Note**: this field
   must be entered as LSBF (meaning it is the reverse value as created in
   LoRa Server)!
3. Enter the *Application Key* as ``01020304050607080910111213141516``.
   **Note:** Opposite to the *Device / Application EUI*, this field must be
   entered as-is (the same value as set in LoRa Server).
4. Click *Set Join Parameter*
5. Click *Join Network*

## Your node not here?

Please create a GitHub issue with instructions how your node should be
provisioned :-)
