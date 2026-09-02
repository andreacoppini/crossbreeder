/** Shapes shared across the SmartZone public API. */

/** Every `GET` collection endpoint answers in this envelope. */
export interface SmartZoneList<T> {
  totalCount: number;
  hasMore: boolean;
  firstIndex: number;
  list: T[];
}

/** Objects the controller returns as `{id, name}` references. */
export interface NamedRef {
  id: string;
  name?: string;
}

/** `POST /serviceTicket` response. */
export interface ServiceTicket {
  serviceTicket: string;
  controllerVersion: string;
}

/** `GET /wsg/api/public/apiInfo` response. */
export interface ApiInfo {
  apiSupportVersions: string[];
  apiVersions?: { version: string }[];
}

/** The live session an authenticated client holds. */
export interface Session {
  serviceTicket: string;
  controllerVersion: string;
  apiVersion: string;
  /** Epoch millis. The controller expires tickets at 24 hours. */
  issuedAt: number;
}

/** Offset pagination, as the `GET` list endpoints spell it. */
export interface ListPage {
  index?: number;
  listSize?: number;
}
