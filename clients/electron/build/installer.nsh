; installer.nsh — Se incluye en el instalador NSIS de electron-builder.
; Registra el host de native messaging de Passtore para Chrome y Edge, apuntando
; al passtore-native.exe empaquetado (en resources\bin), y lo elimina al desinstalar.
;
; IMPORTANTE: reemplaza __CHROME_EXTENSION_ID__ y __EDGE_EXTENSION_ID__ por los IDs
; reales de la extensión en la Chrome Web Store y Edge Add-ons tras el primer envío.

!macro customInstall
  ; Manifiesto del host nativo. La ruta del binario es RELATIVA al directorio del
  ; manifiesto (Chrome lo permite en Windows), así evitamos escapar rutas absolutas.
  FileOpen $0 "$INSTDIR\com.passtore.host.json" w
  FileWrite $0 '{$\n'
  FileWrite $0 '  "name": "com.passtore.host",$\n'
  FileWrite $0 '  "description": "Passtore native messaging host",$\n'
  FileWrite $0 '  "path": "resources\\bin\\passtore-native.exe",$\n'
  FileWrite $0 '  "type": "stdio",$\n'
  FileWrite $0 '  "allowed_origins": [$\n'
  FileWrite $0 '    "chrome-extension://__CHROME_EXTENSION_ID__/",$\n'
  FileWrite $0 '    "chrome-extension://__EDGE_EXTENSION_ID__/"$\n'
  FileWrite $0 '  ]$\n'
  FileWrite $0 '}$\n'
  FileClose $0

  WriteRegStr HKCU "Software\Google\Chrome\NativeMessagingHosts\com.passtore.host" "" "$INSTDIR\com.passtore.host.json"
  WriteRegStr HKCU "Software\Microsoft\Edge\NativeMessagingHosts\com.passtore.host" "" "$INSTDIR\com.passtore.host.json"
!macroend

!macro customUnInstall
  DeleteRegKey HKCU "Software\Google\Chrome\NativeMessagingHosts\com.passtore.host"
  DeleteRegKey HKCU "Software\Microsoft\Edge\NativeMessagingHosts\com.passtore.host"
  Delete "$INSTDIR\com.passtore.host.json"
!macroend
