# Activating nodes

After setting up an application and node in LoRa Server
(see [getting started](getting-started.md) for more details), you need to
provision your node with the chosen AppEUI, DevEUI and AppKey.
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
with a node running v1.14. Use [WiMOD LoRaWAN EndNode Studio](http://www.wireless-solutions.de/products/radiomodules/im880b-l)
for the following actions:

### To set the DevEUI

1. Go to **Extras** -> **Factory Settings**
2. Click **Set Customer Mode**
3. Enter the **Device EUI** as ``0807060504030201``. **Note**: this field
   must be entered as LSBF (meaning it is the reverse value as created in
   LoRa Server)!
4. Click **Set Device EUI**
5. Click **Set Application Mode**

### To set the AppEUI and AppKey

1. Go back to **LoRaWAN Node**
2. Enter the **Application EUI** as ``0102030405060708``. **Note**: this field
   must be entered as LSBF (meaning it is the reverse value as created in
   LoRa Server)!
3. Enter the **Application Key** as ``01020304050607080910111213141516``.
   **Note:** Opposite to the *Device / Application EUI*, this field must be
   entered as-is (the same value as set in LoRa Server).
4. Click **Set Join Parameter**
5. Click **Join Network**

## MultiConnect® mDot™
  Through a serial terminal, use the following commands:
  
  ```
  AT
  AT+NJM=1
  AT+NI=0,0807060504030201
  AT+NK=0,01020304050607080910111213141516
  AT&W

  ATZ

  AT+JOIN
  ```

  For mDot™ we can't modify DevEUI, it's a factory-programmed setting
  use the following command to obtain it
  
  ```
  AT+DI?
  ```

!!! info "Your node not here?"
    Please help making this guide complete! Fork the [github.com/brocaar/loraserver](https://github.com/brocaar/loraserver)
    repository, update this page with the actions needed to setup your node
    and create a pull-request :-)
