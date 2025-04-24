# Changelog

## 2.9.0 / 2025-04-24

* [Added] Upgrade Go to 1.24.2. See [#123](https://github.com/DataDog/datadog-firehose-nozzle/pull/123).
* [Added] Add support for `container_id` tag in logs. See [#122](https://github.com/DataDog/datadog-firehose-nozzle/pull/122).

## 2.8.0 / 2025-01-14

* [Added] Add metric origin metadata. See [#114](https://github.com/DataDog/datadog-firehose-nozzle/pull/114).
* [Added] Upgrade Go to 1.21.5. See [#112](https://github.com/DataDog/datadog-firehose-nozzle/pull/112).

## 2.7.0 / 2023-11-13

* [Added] Add support for logs and traces correlation. See [#109](https://github.com/DataDog/datadog-firehose-nozzle/pull/109).
* [Added] Upgrade go to 1.21. See [#104](https://github.com/DataDog/datadog-firehose-nozzle/pull/104).
* [Fixed] Fix `results-per-page` parameter to not exceed the maximum limit in the V2 API. See [#108](https://github.com/DataDog/datadog-firehose-nozzle/pull/108).
* [Fixed] Increase wait time before refreshing UAA tokens. See [#103](https://github.com/DataDog/datadog-firehose-nozzle/pull/103).

## 2.6.0 / 2023-09-08

* [Added] Add support for application logs. See [#92](https://github.com/DataDog/datadog-firehose-nozzle/pull/92).

## 2.5.0 / 2022-11-10

* [Added] Add option to disable application metadata prefix. See [#93](https://github.com/DataDog/datadog-firehose-nozzle/pull/93).
* [Fixed] Fix SSL errors when using the Cluster Agent API. See [#96](https://github.com/DataDog/datadog-firehose-nozzle/pull/96).

## 2.4.1 / 2022-04-27

* [Fixed] Fix sidecars tagging. See [#90](https://github.com/DataDog/datadog-firehose-nozzle/pull/90).

## 2.4.0 / 2022-04-13

* [Added] Add sidecars tags to app metrics. See [#88](https://github.com/DataDog/datadog-firehose-nozzle/pull/88).
* [Added] Automatically add tags to orgs metrics based on metadata. See [#87](https://github.com/DataDog/datadog-firehose-nozzle/pull/87).
* [Added] Add support for the DCA API. See [#86](https://github.com/DataDog/datadog-firehose-nozzle/pull/86).
* [Added] Improve telemetry of the nozzle. See [#85](https://github.com/DataDog/datadog-firehose-nozzle/pull/85).
* [Added] Improve metric submission logic to split metric packets. See [#84](https://github.com/DataDog/datadog-firehose-nozzle/pull/84).

## 2.3.0 / 2021-08-05

* [Added] Automatically add tags to application metrics based on metadata. See [#81](https://github.com/DataDog/datadog-firehose-nozzle/pull/81).
* [Added] Upgrade go version to 1.16 and move to go modules. See [#82](https://github.com/DataDog/datadog-firehose-nozzle/pull/82).

## 2.2.0 / 2020-07-23

* [Added] Add labels and annotations as tags. See [#79](https://github.com/DataDog/datadog-firehose-nozzle/pull/79).
