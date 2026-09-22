<?xml version="1.0" encoding="UTF-8"?>
<tileset version="1.10" tiledversion="1.12.2" name="region" tilewidth="32" tileheight="32" tilecount="8" columns="8">
 <image source="region.png" width="256" height="32"/>
 <tile id="0">
  <properties>
   <property name="food" type="float" value="0.6"/>
   <property name="region" value="east"/>
   <property name="rich" type="float" value="0.6"/>
   <property name="shelter" type="float" value="0.8"/>
  </properties>
 </tile>
 <tile id="1">
  <properties>
   <property name="food" type="float" value="1.6"/>
   <property name="region" value="west"/>
   <property name="rich" type="float" value="1.6"/>
   <property name="shelter" type="float" value="1.3"/>
  </properties>
 </tile>
 <tile id="2">
  <properties>
   <property name="enemies" type="float" value="1.5"/>
   <property name="food" type="float" value="1"/>
   <property name="goal" type="bool" value="true"/>
   <property name="region" value="south"/>
   <property name="rich" type="float" value="1"/>
  </properties>
 </tile>
 <tile id="3">
  <properties>
   <property name="goal" type="bool" value="true"/>
   <property name="price" type="int" value="5"/>
   <property name="region" value="goal1"/>
   <property name="years" type="float" value="10"/>
  </properties>
 </tile>
 <tile id="4">
  <properties>
   <property name="goal" type="bool" value="true"/>
   <property name="price" type="int" value="10"/>
   <property name="region" value="goal2"/>
   <property name="years" type="float" value="10"/>
  </properties>
 </tile>
 <tile id="5">
  <properties>
   <property name="goal" type="bool" value="true"/>
   <property name="price" type="int" value="15"/>
   <property name="region" value="goal3"/>
   <property name="years" type="float" value="10"/>
  </properties>
 </tile>
</tileset>
