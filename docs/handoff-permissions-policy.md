# Handoff: Permissions-Policy allows camera/mic for Loom

## Why
Loom's Chrome extension could not use the camera or microphone on
https://tally-polls-live.vercel.app. The site sent `camera=()` and `microphone=()`,
which disables both for the whole page, including extension frames.

## File changed
`frontend/vercel.json` → `headers` → `"source": "/(.*)"` → `Permissions-Policy`.
This is the only place the header is set (the Go API, Vite config and React app do not send it).

## Old value
```
camera=(), microphone=(), geolocation=()
```

## New value
```
camera=(self "chrome-extension://liecbddmkiiihnedobmlmillhodjkdmb"), microphone=(self "chrome-extension://liecbddmkiiihnedobmlmillhodjkdmb"), geolocation=()
```
Camera and mic are allowed for the site itself and the Loom extension only. Geolocation stays blocked.
In `vercel.json` the inner quotes are escaped as `\"`.

No other security headers changed (`X-Frame-Options`, `X-Content-Type-Options`, `Referrer-Policy`,
the API's HSTS and CORS are untouched).

## Verify after deploying
```bash
curl -sI https://tally-polls-live.vercel.app | grep -i permissions-policy
```
should print the new value. In the browser console on the site:
```js
document.featurePolicy.allowsFeature('camera')      // true
document.featurePolicy.allowsFeature('geolocation') // false
```
Then start a Loom recording with camera on to confirm.
