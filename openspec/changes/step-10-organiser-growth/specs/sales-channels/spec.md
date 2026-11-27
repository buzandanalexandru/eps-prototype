# Spec Delta

## Purpose

Lets one event be sold through several shops (presale, VIP, partner or marketplace channels), each with its own products and rules, while all of them draw on the same capacity.

## ADDED Requirements

### Requirement: Several shops per event
The system SHALL let an organiser add shops to an event, each with a name, a path `/s/<brand>/<shop>`, its own subset of the event's products and an optional access-code requirement, and SHALL publish them with the event's sale snapshot.

#### Scenario: VIP shop on the same event
- **WHEN** the organiser adds a VIP shop with only the VIP products and publishes
- **THEN** `/s/<brand>/vip` shows only the VIP products with all-in prices, and the main shop is unchanged
- Test: apps/web/tests/e2e/sales-channels.spec.ts

### Requirement: Shops share capacity
The system SHALL sell products of every shop from the same capacity pools, so a sale in one shop reduces the stock available in the others.

#### Scenario: Pool sold out through the partner shop
- **WHEN** the last tickets of a pool are sold through the partner shop
- **THEN** products of that pool show sold out in every shop of the event
- Test: services/core/internal/channels/channels_test.go

### Requirement: Access-only shop
The system SHALL refuse to show products or take holds in a shop that requires an access code until a valid access code for that shop is applied.

#### Scenario: Partner shop without a code
- **WHEN** a fan opens an access-only partner shop without a code
- **THEN** the shop asks for the code and a hold request for its products is refused
- Test: services/core/internal/channels/channels_test.go

### Requirement: Channel recorded on orders
The system SHALL record the shop on every order and show sales per shop on the sales dashboard.

#### Scenario: Sales split by shop
- **WHEN** orders are paid in the main shop and the VIP shop
- **THEN** the dashboard shows the orders and revenue of each shop separately
- Test: manual check on the deployed preview (Sales screen shows a per-shop breakdown)
