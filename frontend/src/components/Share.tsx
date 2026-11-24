import { useEffect, useRef, useState } from 'react'
import QRCode from 'qrcode'

export function QR({ value }: { value: string }) {
  const [svg, setSvg] = useState('')
  useEffect(() => {
    let live = true
    QRCode.toString(value, { type: 'svg', margin: 0, errorCorrectionLevel: 'M', color: { dark: '#16130f', light: '#ffffff' } })
      .then((s) => live && setSvg(s))
      .catch(() => live && setSvg(''))
    return () => {
      live = false
    }
  }, [value])
  // The SVG comes from the qrcode library for our own URL, not from user input.
  return <div className="qr" role="img" aria-label={`QR code for ${value}`} dangerouslySetInnerHTML={{ __html: svg }} />
}

export function ShareBox({ url, showQR = true }: { url: string; showQR?: boolean }) {
  const [copied, setCopied] = useState(false)
  const input = useRef<HTMLInputElement>(null)

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(url)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      // Clipboard API is unavailable on some browsers; select the text so the user can copy it.
      input.current?.select()
    }
  }

  return (
    <div className="share">
      <span className="stat label">Share link</span>
      <div className="url">
        <input ref={input} type="text" readOnly value={url} aria-label="Poll link" onFocus={(e) => e.target.select()} />
        <button type="button" className="btn small" onClick={copy}>
          {copied ? 'Copied' : 'Copy'}
        </button>
      </div>
      {showQR && <QR value={url} />}
    </div>
  )
}
