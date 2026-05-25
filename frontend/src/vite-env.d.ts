/// <reference types="vite/client" />

interface Window {
  go: {
    main: {
      App: {
        ListAccounts: () => Promise<any[]>
        AddAccountByPAT: (pat: string, region: string) => Promise<any>
        StartOAuthLogin: (region: string) => Promise<{ login_id: string; login_url: string }>
        WaitOAuthLogin: (loginID: string) => Promise<void>
        CancelOAuthLogin: (loginID: string) => Promise<void>
        DeleteAccount: (id: string) => Promise<void>
        SetActiveAccount: (id: string) => Promise<void>
        GetStatus: () => Promise<{ running: boolean; port: number; active_account: string }>
        StartBridge: () => Promise<void>
        StopBridge: () => Promise<void>
      }
    }
  }
  runtime: {
    BrowserOpenURL: (url: string) => void
    EventsOn: (event: string, callback: (...args: any[]) => void) => void
    EventsOff: (...events: string[]) => void
  }
}
