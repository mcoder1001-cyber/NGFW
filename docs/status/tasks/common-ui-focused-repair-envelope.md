# Focused Common UI repair task

Repair the seven independently reproduced compatibility findings without migrating NGFW from React Hook Form/SchemaForm to usePgForm. Work in the existing original library integration and NGFW migration branch; preserve all preexisting changes. No Common UI commit until explicitly requested. User requested implementation first and tests as final phase.

Review scope: controlled ownership, hidden validation, read-only checks, chip array draft validation, controlled grid filters, client mode processing, yalc freshness. Regression tests must distinguish library source from the actual NGFW installed distribution. Independent reviewer is read-only. Real API/device acceptance remains separate.
