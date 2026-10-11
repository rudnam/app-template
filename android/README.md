# Android app

The web app is already installable on Android: open the site in Chrome and choose **Add to Home screen**. That needs no extra code, and is enough for personal use.

To ship it on the **Play Store**, wrap the site in a **Trusted Web Activity (TWA)**: a small Android app that opens your site full screen in Chrome, without browser UI. Because the site is the app:

- every web deploy updates the Android app too, with no store review;
- sign-in, cookies and the API work exactly as on the web; nothing in the backend changes.

The cost: it needs a network connection, and it can't use native APIs that the web can't. If you later need those (offline use, background work, deep device access), the next step is Capacitor or a native Kotlin app that calls the same `/api`.

## Create the app (once)

You need a JDK (17+). Bubblewrap downloads the Android SDK itself the first time.

```sh
cd android
npx @bubblewrap/cli init --manifest https://<your-domain>/manifest.webmanifest
#   package id: e.g. com.yourname.yourapp. It can't change after you publish.
#   It also creates a signing key (android.keystore). Back it up somewhere safe
#   and never commit it (.gitignore excludes it): without it you can't publish updates.
npx @bubblewrap/cli build      # makes app-release-signed.apk and app-release-bundle.aab
```

Commit the generated project (`twa-manifest.json`, the Gradle files and `app/`). Build output and the key are ignored.

## Prove the site and the app belong together

Without this step, the app opens with a browser address bar.

1. Get the key's SHA-256 fingerprint: `keytool -list -v -keystore android.keystore` (the `SHA256:` line). If Google Play signs your app (the default), also add the fingerprint from **Play Console → Setup → App signing**.
2. On the server, in `.env`:
   ```sh
   ANDROID_PACKAGE=com.yourname.yourapp
   ANDROID_CERT_FINGERPRINTS=AA:BB:...,CC:DD:...   # comma-separated
   ```
3. `docker compose up -d`, then check that `https://<your-domain>/.well-known/assetlinks.json` lists them.

## Test and publish

- Install on a phone with USB debugging on: `npx @bubblewrap/cli install`.
- Upload `app-release-bundle.aab` in the Play Console.
- After changing the manifest (name, colors, icons), run `npx @bubblewrap/cli update` and build again. Changes to the site itself need no new build.
