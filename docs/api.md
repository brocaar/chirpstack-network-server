# API

LoRa Server provides a [JSON-RPC (v1.0)](https://en.wikipedia.org/wiki/JSON-RPC#Version_1.0)
API so that you can integrate the server into your own platform.
Documentation of all the available methods is available in the web-interface
(e.g. [http://localhost:8000/](http://localhost:8000/)).
The endpoint of the RPC handler is `/rpc`.

## Examples

To create an application with AppEUI `0102030405060708``, post the following
body to ``/rpc``:

```json
{
    "method": "Application.Create",
    "params": [
        {
            "appEUI": "0102030405060708",
            "name": "test application"
        }
    ],
    "id": 1
}
```
